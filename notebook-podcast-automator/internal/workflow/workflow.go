package workflow

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	pb "notebook-podcast-automator/gen/notebooklm/v1alpha1"
	"notebook-podcast-automator/internal/api"
	"notebook-podcast-automator/internal/auth"
	"notebook-podcast-automator/internal/batchexecute"
	"notebook-podcast-automator/internal/cleaner"
	"notebook-podcast-automator/internal/state"
	"notebook-podcast-automator/internal/uploader"
)

type ProgressFunc func(stage, message string)

func (p ProgressFunc) Report(stage, message string) {
	if p == nil {
		return
	}
	p(stage, message)
}

type Config struct {
	InputURL       string
	MaxEntries     int
	AudioPrompt    string
	ProjectEmoji   string
	PollInterval   time.Duration
	PollTimeout    time.Duration
	DownloadsDir   string
	KeepLocalAudio bool

	StatePath       string
	StateMaxEntries int

	FilterMode            string
	FilterBlockKeywords   []string
	FilterAllowKeywords   []string
	FilterMinContentChars int
	FilterStrict          bool
	FilterLLMBaseURL      string
	FilterLLMModel        string
	FilterLLMTitleModel   string
	FilterLLMMaxChars     int
	FilterLLMTimeout      time.Duration
	FilterLLMRetries      int
	FilterLLMAPIKey       string
}

type Source struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Key     string `json:"key,omitempty"`
	Content string `json:"-"`
}

type Result struct {
	Noop         bool      `json:"noop,omitempty"`
	PodcastTitle string    `json:"podcast_title"`
	Summary      string    `json:"summary"`
	GeneratedAt  time.Time `json:"generated_at"`

	ProjectID string `json:"project_id"`

	AudioTitle     string `json:"audio_title,omitempty"`
	AudioLocalPath string `json:"audio_local_path,omitempty"`
	AudioURL       string `json:"audio_url,omitempty"`
	AudioSizeBytes int64  `json:"audio_size_bytes,omitempty"`

	RSSFeedURL string `json:"rss_feed_url,omitempty"`

	Sources []Source `json:"sources"`
}

func Run(ctx context.Context, cfg Config, progress ProgressFunc) (Result, error) {
	cfg = withDefaults(cfg)
	if cfg.InputURL == "" {
		return Result{}, fmt.Errorf("input_url required")
	}
	now := time.Now().In(beijingLocation())

	progress.Report("input", "fetching input feed/article")
	inputBytes, err := fetchBytes(ctx, cfg.InputURL)
	if err != nil {
		return Result{}, err
	}

	isFeed := looksLikeFeed(inputBytes)
	var feedTitle string

	var candidates []Source
	var st *state.Store
	if isFeed {
		parsedTitle, feedItems, _, parseErr := ParseFeed(inputBytes)
		if parseErr != nil {
			return Result{}, fmt.Errorf("parse feed: %w", parseErr)
		}
		feedTitle = parsedTitle

		if strings.TrimSpace(cfg.StatePath) != "" {
			if opened, err := state.Open(cfg.StatePath, cfg.StateMaxEntries); err != nil {
				progress.Report("warn", fmt.Sprintf("state disabled: %v", err))
			} else {
				st = opened
			}
		}

		selected, skipped, selErr := selectNewCandidatesFromFeed(feedItems, cfg.MaxEntries, st)
		if selErr != nil {
			progress.Report("warn", fmt.Sprintf("state read failed (disable state): %v", selErr))
			st = nil
			selected, skipped, selErr = selectNewCandidatesFromFeed(feedItems, cfg.MaxEntries, nil)
			if selErr != nil {
				return Result{}, selErr
			}
		}
		candidates = selected
		if skipped > 0 {
			progress.Report("dedup", fmt.Sprintf("dedup: new=%d skipped=%d", len(candidates), skipped))
		}
		if len(candidates) == 0 {
			return Result{Noop: true, GeneratedAt: now}, nil
		}
	} else {
		candidates = []Source{{URL: cfg.InputURL, Key: entryKey("", cfg.InputURL)}}
	}

	if mode := normalizeFilterMode(cfg.FilterMode); mode == filterModeRules || mode == filterModeHybrid {
		kept, dropped := prefilterCandidatesByRules(candidates, cfg, func(src Source, reason string) {
			if st == nil {
				return
			}
			_ = st.MarkSkipped(src.Key, src.URL, src.Title, reason)
		})
		if dropped > 0 {
			progress.Report("filter", fmt.Sprintf("prefiltered candidates: kept=%d dropped=%d", len(kept), dropped))
		}
		if len(kept) == 0 {
			if cfg.FilterStrict {
				return Result{}, fmt.Errorf("no candidates left after prefiltering")
			}
			return Result{Noop: true, GeneratedAt: now}, nil
		}
		candidates = kept
	}

	progress.Report("extract", fmt.Sprintf("extracting content (count=%d)", len(candidates)))
	sources, err := extractSources(candidates, progress, func(src Source, reason string) {
		if st == nil {
			return
		}
		_ = st.MarkSkipped(src.Key, src.URL, src.Title, reason)
	})
	if err != nil {
		return Result{}, err
	}

	if len(sources) == 0 {
		// Extraction can fail for valid-looking entries (e.g. WeChat deleted/blocked pages).
		// When state is enabled, mark such entries as skipped and treat the run as a noop.
		if st != nil {
			return Result{Noop: true, GeneratedAt: now}, nil
		}
		return Result{}, fmt.Errorf("no valid content found to process")
	}

	preferredTitle := ""
	if filtered, title, ferr := filterSources(ctx, sources, cfg, progress, func(src Source, reason string) {
		if st == nil {
			return
		}
		_ = st.MarkSkipped(src.Key, src.URL, src.Title, reason)
	}); ferr != nil {
		return Result{}, ferr
	} else {
		sources = filtered
		preferredTitle = compactWhitespace(title)
	}

	if len(sources) == 0 {
		return Result{Noop: true, GeneratedAt: now}, nil
	}

	podcastTitle := sources[0].Title
	if isFeed {
		podcastTitle = fmt.Sprintf("每日简报 %s", now.Format("2006-01-02"))
		if strings.TrimSpace(feedTitle) != "" {
			podcastTitle = fmt.Sprintf("%s %s", strings.TrimSpace(feedTitle), now.Format("2006-01-02"))
		}
	}

	episode := Result{
		PodcastTitle: podcastTitle,
		GeneratedAt:  now,
		Sources:      sources,
	}
	episode.Summary = buildSummary(sources)

	progress.Report("auth", "ensuring NotebookLM credentials")
	ensureCfg := auth.DefaultEnsureConfig()
	ensureCfg.WriteDotenv = false
	ensureCfg.WriteNlmEnv = false
	creds, err := auth.EnsureCredentials(ensureCfg)
	if err != nil {
		return Result{}, fmt.Errorf("auth failed: %w", err)
	}

	progress.Report("notebooklm", "creating NotebookLM project")
	nlmTransport := http.DefaultTransport.(*http.Transport).Clone()
	nlmHTTPClient := &http.Client{
		Timeout:   90 * time.Second,
		Transport: nlmTransport,
	}
	client := api.New(creds.AuthToken, creds.Cookies, batchexecute.WithHTTPClient(nlmHTTPClient))
	notebook, err := client.CreateProject(episode.PodcastTitle, cfg.ProjectEmoji)
	if err != nil {
		return Result{}, err
	}
	episode.ProjectID = notebook.ProjectId

	deleteNotebookOnExit := false
	deleteNotebookReason := ""
	defer func() {
		if episode.ProjectID == "" {
			return
		}
		if !deleteNotebookOnExit {
			progress.Report("cleanup", fmt.Sprintf("keeping temporary NotebookLM project for debugging (project_id=%s)", episode.ProjectID))
			return
		}
		msg := fmt.Sprintf("deleting temporary NotebookLM project (project_id=%s)", episode.ProjectID)
		if strings.TrimSpace(deleteNotebookReason) != "" {
			msg = msg + " reason=" + strings.TrimSpace(deleteNotebookReason)
		}
		progress.Report("cleanup", msg)
		if err := client.DeleteProjects([]string{episode.ProjectID}); err != nil {
			progress.Report("warn", fmt.Sprintf("failed to delete notebook (project_id=%s): %v", episode.ProjectID, err))
		}
	}()

	progress.Report("notebooklm", "uploading sources")
	for i, src := range sources {
		progress.Report("notebooklm", fmt.Sprintf("uploading source %d/%d", i+1, len(sources)))
		content := strings.TrimSpace(src.Content)
		if strings.TrimSpace(src.URL) != "" {
			content = content + "\n\n来源：" + strings.TrimSpace(src.URL)
		}
		if _, uploadErr := client.AddSourceFromText(episode.ProjectID, content, src.Title); uploadErr != nil {
			progress.Report("warn", fmt.Sprintf("upload failed: %v", uploadErr))
		}
		time.Sleep(1 * time.Second)
	}

	progress.Report("notebooklm", "requesting audio overview")
	if _, err := client.CreateAudioOverview(episode.ProjectID, cfg.AudioPrompt); err != nil {
		progress.Report("warn", fmt.Sprintf("CreateAudioOverview failed: %v", err))
	}

	audioPath, audioSize, audioTitle, err := waitAndDownloadAudio(ctx, client, episode.ProjectID, preferredTitle, episode.PodcastTitle, episode.GeneratedAt, cfg, progress)
	if err != nil {
		return Result{}, err
	}
	episode.AudioTitle = strings.TrimSpace(audioTitle)
	episode.AudioLocalPath = audioPath
	episode.AudioSizeBytes = audioSize

	// Upload to R2 if configured.
	if os.Getenv("R2_ACCOUNT_ID") == "" {
		progress.Report("r2", "R2 not configured; skipping upload and RSS update")
		if st != nil {
			for _, src := range sources {
				_ = st.MarkProcessed(src.Key, src.URL, src.Title, "", "")
			}
		}
		deleteNotebookOnExit = true
		deleteNotebookReason = "local_output_ready"
		return episode, nil
	}

	progress.Report("r2", "uploading audio to Cloudflare R2")
	r2, err := uploader.NewR2Uploader()
	if err != nil {
		return Result{}, err
	}

	objectKey := fmt.Sprintf("episodes/%s", filepath.Base(audioPath))
	episode.AudioURL, err = r2.UploadFile(ctx, audioPath, objectKey)
	if err != nil {
		return Result{}, err
	}
	deleteNotebookOnExit = true
	deleteNotebookReason = "r2_upload_succeeded"

	progress.Report("r2", "updating RSS feed")
	itemTitle := episode.PodcastTitle
	if strings.TrimSpace(episode.AudioTitle) != "" {
		itemTitle = strings.TrimSpace(episode.AudioTitle)
	}
	newItem := uploader.Item{
		Title:          itemTitle,
		Description:    episode.Summary,
		PubDate:        episode.GeneratedAt.Format(time.RFC1123Z),
		Guid:           episode.AudioURL,
		ItunesExplicit: "no",
		Enclosure: uploader.Enclosure{
			URL:    episode.AudioURL,
			Length: episode.AudioSizeBytes,
			Type:   uploader.ContentTypeForPath(audioPath),
		},
	}
	if err := r2.UpdateRSS(ctx, "feed.xml", newItem); err != nil {
		return Result{}, err
	}
	if public := strings.TrimRight(os.Getenv("R2_PUBLIC_URL"), "/"); public != "" {
		episode.RSSFeedURL = public + "/feed.xml"
	}

	if !cfg.KeepLocalAudio {
		_ = os.Remove(audioPath)
		episode.AudioLocalPath = ""
	}

	if st != nil {
		for _, src := range sources {
			_ = st.MarkProcessed(src.Key, src.URL, src.Title, episode.AudioURL, episode.AudioURL)
		}
	}

	return episode, nil
}

func withDefaults(cfg Config) Config {
	if cfg.MaxEntries == 0 {
		cfg.MaxEntries = envInt("NPA_MAX_ENTRIES", 3)
	}
	if cfg.MaxEntries < 0 {
		cfg.MaxEntries = 3
	}
	if strings.TrimSpace(cfg.AudioPrompt) == "" {
		now := time.Now().In(beijingLocation())
		bjDate := now.Format("2006-01-02")
		weekday := []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}[now.Weekday()]
		dayPart := "白天"
		switch h := now.Hour(); {
		case h <= 5:
			dayPart = "凌晨"
		case h <= 11:
			dayPart = "早上"
		case h <= 13:
			dayPart = "中午"
		case h <= 17:
			dayPart = "下午"
		default:
			dayPart = "晚上"
		}
		bjNow := now.Format("2006-01-02 15:04:05")
		cfg.AudioPrompt = strings.TrimSpace(fmt.Sprintf(`当前北京时间：%s（UTC+8），今天是 %s %s %s。如果你在节目中提及“今天/日期/星期”，必须以此为准，不要自行推断时区。

【重要】：开场寒暄要有即时感，可以参考“%s的%s”来组织一句口语化开场，但不要像新闻联播一样精确报时（不要念“15:04:05”这种秒级时间）。

请生成一段中文“对话式”的深度教育播客音频脚本，节目名为《预见未来》，目标听众是一线教师与教育管理者。

【角色设定】
- 女主持人（感性、一线视角）：前班主任，说话温暖细腻，关注微观教学细节与师生情感。她代表老师听众，对宏大概念本能怀疑，常问“这到底怎么落地？”“会不会增加老师负担？”。
- 男主持人（理性、宏观视角）：资深教育观察员，逻辑清晰，擅长用比喻把政策/理论讲人话；负责把话题拔高，但也会被女主持“拽回地面”。

【对话风格（硬约束）】
1. 拒绝“播音腔”，要“人话感”：多用口语连接词（如“其实吧”“说白了”“哎对了”），允许自然感叹（如“哇”“太真实了”“唉，确实难”），句子要短，允许反问与自问自答。
2. 要有真实碰撞：必须有追问、补充、温和反驳（如“理是对的，但现实里……”），不要变成 A 说观点 B 全程附和。允许“捧哏式”简短反馈（如“嗯嗯”“扎心了”“这倒是”）。
3. 情绪要有起伏：轻松开场切入；谈到痛点时有共鸣与无奈；出现精彩案例时有兴奋与感动；结尾要有余韵与思考。
4. 不要写括号动作描写：禁止出现（笑）（叹气）等括号内容；用“哈哈”“唉”“哎哟”等语气词体现情绪，方便语音合成。

【内容深度指令】
- 破除“正确的废话”：出现“顶层设计”“学校治理”等大词时，女主持必须立刻打断或追问：“这听起来太虚了，换成人话说，对老师意味着什么？”
- 从痛点出发：不要先讲理论，先讲老师的真实困境（例如行政琐事缠身、面对 AI 的焦虑、质量分析会的压抑），再把素材中的观点/方案当作“解药”来讨论其可行性与代价。
- 案例要还原现场：引用素材中的案例时，不要念简介，要像讲故事一样描述画面与细节，并点出“为什么这招能用/在什么条件下会翻车”。
- 禁止“念经式”照读：素材里若有晦涩定义、长排比句或官方话术，严禁原句复述；男主持必须用“打比方”重述，女主持可以吐槽“这也太绕了吧”，并追问落地含义。

【播客结构】
1. 开场（30~60 秒）：自然寒暄，带出《预见未来》栏目名，点出今天的话题背景（例如“最近大家是不是都被各种新概念轰炸晕了？”）。
2. 核心讨论：围绕素材推进“一问一答 → 案例佐证 → 延伸思考”，把宏观趋势翻译成具体的“避坑指南/行动建议”（面向老师与管理者分别给建议）。
3. 结语（30~60 秒）：温暖有力，总结 2~3 条可执行建议，给信心但不灌鸡汤，不制造额外负担。

【特别约束】
- 本期节目的所有讨论话题、案例与观点，必须严格基于【已上传的源文档】；严禁引用源文档之外的外部信息。
- 若源文档未提及某方面具体数据或结论，请直接基于源文档内容做定性分析，或跳过该细节；严禁使用“相关资料显示”等模糊话术进行填充，以免产生误导。

【输出格式】
- 只输出对话正文，按“女：…”“男：…”逐行输出；全程中文。`, bjNow, bjDate, weekday, dayPart, weekday, dayPart))
	}
	if strings.TrimSpace(cfg.ProjectEmoji) == "" {
		cfg.ProjectEmoji = "🗞️"
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 10 * time.Second
	}
	if cfg.PollTimeout <= 0 {
		cfg.PollTimeout = 20 * time.Minute
	}
	if strings.TrimSpace(cfg.DownloadsDir) == "" {
		cfg.DownloadsDir = "downloads"
	}
	if strings.TrimSpace(cfg.StatePath) == "" {
		cfg.StatePath = strings.TrimSpace(os.Getenv("NPA_STATE_PATH"))
	}
	if strings.TrimSpace(cfg.StatePath) == "" {
		cfg.StatePath = "data/state.json"
	}
	if envBool("NPA_STATE_DISABLED", false) {
		cfg.StatePath = ""
	}
	if cfg.StateMaxEntries == 0 {
		cfg.StateMaxEntries = envInt("NPA_STATE_MAX_ENTRIES", 100)
	}
	if cfg.StateMaxEntries <= 0 {
		cfg.StateMaxEntries = 100
	}
	if cfg.StateMaxEntries > 100 {
		cfg.StateMaxEntries = 100
	}

	explicitFilterMode := strings.TrimSpace(cfg.FilterMode) != ""
	if strings.TrimSpace(cfg.FilterMode) == "" {
		cfg.FilterMode = strings.TrimSpace(os.Getenv("NPA_FILTER_MODE"))
	}
	cfg.FilterMode = normalizeFilterMode(cfg.FilterMode)
	if cfg.FilterBlockKeywords == nil {
		if v := strings.TrimSpace(os.Getenv("NPA_FILTER_BLOCK_KEYWORDS")); v != "" {
			cfg.FilterBlockKeywords = splitCommaList(v)
		}
	}
	if cfg.FilterAllowKeywords == nil {
		if v := strings.TrimSpace(os.Getenv("NPA_FILTER_ALLOW_KEYWORDS")); v != "" {
			cfg.FilterAllowKeywords = splitCommaList(v)
		}
	}
	if cfg.FilterMinContentChars <= 0 && !explicitFilterMode {
		cfg.FilterMinContentChars = envInt("NPA_FILTER_MIN_CONTENT_CHARS", 0)
	}
	if !cfg.FilterStrict && !explicitFilterMode && envBool("NPA_FILTER_STRICT", false) {
		cfg.FilterStrict = true
	}
	if cfg.FilterLLMTimeout <= 0 {
		cfg.FilterLLMTimeout = time.Duration(envInt("NPA_FILTER_LLM_TIMEOUT_SECONDS", 30)) * time.Second
	}
	if cfg.FilterLLMRetries == 0 {
		cfg.FilterLLMRetries = envInt("NPA_FILTER_LLM_RETRIES", 2)
	}
	if cfg.FilterLLMRetries < 0 {
		cfg.FilterLLMRetries = 0
	}
	if cfg.FilterLLMRetries > 10 {
		cfg.FilterLLMRetries = 10
	}
	// 0 means "no truncation" (send full cleaned content to the LLM).
	// Only apply truncation when a positive limit is explicitly provided.
	if cfg.FilterLLMMaxChars == 0 {
		cfg.FilterLLMMaxChars = envInt("NPA_FILTER_LLM_MAX_CHARS", 0)
	}
	if strings.TrimSpace(cfg.FilterLLMBaseURL) == "" {
		cfg.FilterLLMBaseURL = strings.TrimSpace(os.Getenv("NPA_FILTER_LLM_BASE_URL"))
	}
	if strings.TrimSpace(cfg.FilterLLMModel) == "" {
		cfg.FilterLLMModel = strings.TrimSpace(os.Getenv("NPA_FILTER_LLM_MODEL"))
	}
	if strings.TrimSpace(cfg.FilterLLMTitleModel) == "" {
		cfg.FilterLLMTitleModel = strings.TrimSpace(os.Getenv("NPA_FILTER_LLM_TITLE_MODEL"))
	}
	if strings.TrimSpace(cfg.FilterLLMAPIKey) == "" {
		cfg.FilterLLMAPIKey = strings.TrimSpace(os.Getenv("NPA_FILTER_LLM_API_KEY"))
		if cfg.FilterLLMAPIKey == "" {
			cfg.FilterLLMAPIKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		}
	}

	return cfg
}

func beijingLocation() *time.Location {
	if loc, err := time.LoadLocation("Asia/Shanghai"); err == nil {
		return loc
	}
	return time.FixedZone("UTC+8", 8*60*60)
}

func looksLikeFeed(b []byte) bool {
	s := string(b)
	if len(s) > 4096 {
		s = s[:4096]
	}
	return strings.Contains(s, "<?xml") || strings.Contains(s, "<feed") || strings.Contains(s, "<rss") || strings.Contains(s, "<entry") || strings.Contains(s, "<channel")
}

func selectNewCandidatesFromFeed(items []FeedItem, max int, st *state.Store) ([]Source, int, error) {
	seen := make(map[string]bool)
	out := make([]Source, 0, len(items))
	skipped := 0

	for _, it := range items {
		u := strings.TrimSpace(it.URL)
		if u == "" {
			continue
		}

		key := entryKey(it.ID, u)
		if key == "" {
			continue
		}
		if seen[key] {
			continue
		}
		seen[key] = true

		if st != nil {
			handled, _, err := st.HasHandled(key)
			if err != nil {
				return nil, 0, err
			}
			if handled {
				skipped++
				continue
			}
		}

		out = append(out, Source{
			Title: strings.TrimSpace(it.Title),
			URL:   u,
			Key:   key,
		})
		if max > 0 && len(out) >= max {
			break
		}
	}

	return out, skipped, nil
}

func entryKey(entryID string, rawURL string) string {
	entryID = strings.TrimSpace(entryID)
	if entryID != "" {
		return "id:" + entryID
	}
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	return "url:" + canonicalizeURLForKey(rawURL)
}

func canonicalizeURLForKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return raw
	}
	u.Fragment = ""
	u.Scheme = strings.ToLower(strings.TrimSpace(u.Scheme))
	u.Host = strings.ToLower(strings.TrimSpace(u.Host))

	host := u.Hostname()
	port := u.Port()
	if port != "" {
		if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
			u.Host = host
		}
	}

	q := u.Query()
	for _, k := range []string{
		"utm_source", "utm_medium", "utm_campaign", "utm_content", "utm_term",
		"spm", "from", "scene", "src", "source",
		"share_token", "share_time", "xshare",
	} {
		q.Del(k)
	}
	u.RawQuery = q.Encode()

	return u.String()
}

func extractSources(candidates []Source, progress ProgressFunc, onSkip func(Source, string)) ([]Source, error) {
	var out []Source
	for i, c := range candidates {
		if strings.TrimSpace(c.URL) == "" {
			continue
		}
		title, content, err := cleaner.ExtractContent(c.URL)
		if err != nil {
			reason := fmt.Sprintf("extract_failed: %v", err)
			if onSkip != nil {
				onSkip(c, reason)
			}
			progress.Report("warn", fmt.Sprintf("extract failed (index=%d/%d title=%s url=%s): %v", i+1, len(candidates), strings.TrimSpace(c.Title), strings.TrimSpace(c.URL), err))
			continue
		}
		if strings.TrimSpace(content) == "" {
			reason := "extract_empty"
			if onSkip != nil {
				onSkip(c, reason)
			}
			progress.Report("warn", fmt.Sprintf("extract empty (index=%d/%d title=%s url=%s)", i+1, len(candidates), strings.TrimSpace(c.Title), strings.TrimSpace(c.URL)))
			continue
		}
		if strings.TrimSpace(title) == "" {
			title = strings.TrimSpace(c.Title)
		}
		if strings.TrimSpace(title) == "" {
			title = fmt.Sprintf("Article %s", time.Now().Format("15:04:05"))
		}
		out = append(out, Source{Title: title, URL: c.URL, Key: c.Key, Content: content})
	}
	return out, nil
}

func buildSummary(sources []Source) string {
	if len(sources) == 1 {
		return fmt.Sprintf("Article: %s", sources[0].Title)
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Digest containing %d articles: ", len(sources))
	for i, s := range sources {
		if i >= 3 {
			break
		}
		b.WriteString(s.Title)
		b.WriteString("; ")
	}
	return b.String()
}

func resolveAudioTitle(client *api.Client, projectID string, overview *api.AudioOverviewResult, preferredTitle string, fallbackTitle string) (title string, from string, err error) {
	if t := strings.TrimSpace(preferredTitle); t != "" {
		return t, "preferred", nil
	}

	var artifactErr error
	if client != nil && overview != nil {
		if id := strings.TrimSpace(overview.AudioID); id != "" {
			t, terr := client.GetArtifactTitle(projectID, id)
			if terr != nil {
				artifactErr = terr
			} else if tt := strings.TrimSpace(t); tt != "" {
				return tt, "artifact_audio_id", nil
			}
		}

		// If overview.AudioID isn't directly fetchable as an artifact, scan project artifacts
		// for the audio overview and use its user-visible note title.
		if strings.TrimSpace(projectID) != "" {
			artifacts, aerr := client.ListArtifacts(projectID)
			if aerr != nil && artifactErr == nil {
				artifactErr = aerr
			}
			for _, a := range artifacts {
				if a == nil {
					continue
				}
				if a.GetType() != pb.ArtifactType_ARTIFACT_TYPE_AUDIO_OVERVIEW {
					continue
				}
				aid := strings.TrimSpace(a.GetArtifactId())
				if aid == "" {
					continue
				}
				t, terr := client.GetArtifactTitle(projectID, aid)
				if terr != nil {
					if artifactErr == nil {
						artifactErr = terr
					}
					continue
				}
				if tt := strings.TrimSpace(t); tt != "" {
					return tt, "artifact_scan", nil
				}
			}
		}
		if t := strings.TrimSpace(overview.Title); t != "" {
			return t, "overview", artifactErr
		}
	}
	if t := strings.TrimSpace(fallbackTitle); t != "" {
		return t, "fallback", artifactErr
	}
	return "", "empty", artifactErr
}

func waitAndDownloadAudio(ctx context.Context, client *api.Client, projectID string, preferredTitle string, fallbackTitle string, ts time.Time, cfg Config, progress ProgressFunc) (string, int64, string, error) {
	ctx, cancel := context.WithTimeout(ctx, cfg.PollTimeout)
	defer cancel()

	if err := os.MkdirAll(cfg.DownloadsDir, 0755); err != nil {
		return "", 0, "", fmt.Errorf("create downloads dir: %w", err)
	}

	tmpPath := filepath.Join(cfg.DownloadsDir, fmt.Sprintf("audio_%s.tmp", ts.Format("20060102-150405")))

	progress.Report("audio", fmt.Sprintf("polling audio overview (interval=%s timeout=%s)", cfg.PollInterval, cfg.PollTimeout))

	attempt := 0
	sleep := time.Duration(0)
	consecutiveUnavailable := 0

	for {
		if sleep > 0 {
			timer := time.NewTimer(sleep)
			select {
			case <-ctx.Done():
				timer.Stop()
				return "", 0, "", fmt.Errorf("audio generation timeout: %w", ctx.Err())
			case <-timer.C:
			}
		} else {
			select {
			case <-ctx.Done():
				return "", 0, "", fmt.Errorf("audio generation timeout: %w", ctx.Err())
			default:
			}
		}

		attempt++

		overview, err := client.GetAudioOverview(projectID)
		if err != nil {
			if isUnavailable(err) {
				consecutiveUnavailable++
				sleep = minDuration(cfg.PollInterval*time.Duration(1<<minInt(consecutiveUnavailable, 6)), 60*time.Second)
				progress.Report("audio", fmt.Sprintf("poll failed (attempt=%d unavailable=%d next=%s): %v", attempt, consecutiveUnavailable, sleep, err))
				continue
			}

			consecutiveUnavailable = 0
			sleep = cfg.PollInterval
			progress.Report("audio", fmt.Sprintf("poll failed (attempt=%d): %v", attempt, err))
			continue
		}

		consecutiveUnavailable = 0
		sleep = cfg.PollInterval

		audioData := strings.TrimSpace(overview.AudioData)
		progress.Report("audio", fmt.Sprintf("poll ok (attempt=%d ready=%v dataLen=%d)", attempt, overview.IsReady, len(audioData)))
		if audioData == "" {
			continue
		}

		audioTitle, titleFrom, titleErr := resolveAudioTitle(client, projectID, overview, preferredTitle, fallbackTitle)
		if titleErr != nil {
			progress.Report("warn", fmt.Sprintf("audio title: artifact lookup failed (artifact_id=%s): %v", shortID(overview.AudioID), titleErr))
		}
		if strings.TrimSpace(audioTitle) != "" {
			progress.Report("audio", fmt.Sprintf("resolved audio title (%s): %s", titleFrom, strings.TrimSpace(audioTitle)))
		}

		if strings.HasPrefix(audioData, "http") {
			progress.Report("audio", "audio is URL; downloading")
			if err := downloadToFile(ctx, tmpPath, audioData); err != nil {
				return "", 0, "", err
			}
			ext, extErr := detectAudioExtFromFile(tmpPath)
			if extErr != nil || strings.TrimSpace(ext) == "" {
				ext = ".mp3"
			}
			outPath := audioOutputPath(cfg.DownloadsDir, audioTitle, fallbackTitle, ts, ext, overview.AudioID)
			progress.Report("audio", fmt.Sprintf("saving as %s", filepath.Base(outPath)))
			if err := os.Rename(tmpPath, outPath); err != nil {
				return "", 0, "", fmt.Errorf("finalize audio file: %w", err)
			}
			path, size, err := fileSize(outPath)
			if err != nil {
				return "", 0, "", err
			}
			return path, size, strings.TrimSpace(audioTitle), nil
		}

		if b, decodeErr := overview.GetAudioBytes(); decodeErr == nil && len(b) > 0 {
			ext := detectAudioExtFromBytes(b)
			outPath := audioOutputPath(cfg.DownloadsDir, audioTitle, fallbackTitle, ts, ext, overview.AudioID)
			progress.Report("audio", fmt.Sprintf("audio is base64 (len=%d ext=%s); writing %s", len(b), ext, filepath.Base(outPath)))
			if err := os.WriteFile(outPath, b, 0644); err != nil {
				return "", 0, "", fmt.Errorf("write audio file: %w", err)
			}
			path, size, err := fileSize(outPath)
			if err != nil {
				return "", 0, "", err
			}
			return path, size, strings.TrimSpace(audioTitle), nil
		}

		if b, decodeErr := decodeMaybeDataURLBase64(audioData); decodeErr == nil && len(b) > 0 {
			ext := detectAudioExtFromBytes(b)
			outPath := audioOutputPath(cfg.DownloadsDir, audioTitle, fallbackTitle, ts, ext, overview.AudioID)
			progress.Report("audio", fmt.Sprintf("audio is data-url base64 (len=%d ext=%s); writing %s", len(b), ext, filepath.Base(outPath)))
			if err := os.WriteFile(outPath, b, 0644); err != nil {
				return "", 0, "", fmt.Errorf("write audio file: %w", err)
			}
			path, size, err := fileSize(outPath)
			if err != nil {
				return "", 0, "", err
			}
			return path, size, strings.TrimSpace(audioTitle), nil
		}

		return "", 0, "", fmt.Errorf("unsupported audio payload format (not url/base64)")
	}
}

func fileSize(path string) (string, int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", 0, err
	}
	return path, info.Size(), nil
}

var invalidFilenameChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1F]`)

func audioOutputPath(downloadsDir string, preferredTitle string, fallbackTitle string, ts time.Time, ext string, audioID string) string {
	ext = strings.TrimSpace(ext)
	if ext == "" {
		ext = ".mp3"
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}

	base := sanitizeFilename(preferredTitle)
	if base == "" {
		base = sanitizeFilename(fallbackTitle)
	}
	if base == "" {
		base = fmt.Sprintf("podcast_%s", ts.Format("20060102-150405"))
	}

	candidate := filepath.Join(downloadsDir, base+ext)
	if !pathExists(candidate) {
		return candidate
	}

	tsSuffix := ts.Format("20060102-150405")
	candidate = filepath.Join(downloadsDir, fmt.Sprintf("%s_%s%s", base, tsSuffix, ext))
	if !pathExists(candidate) {
		return candidate
	}

	if short := shortID(audioID); short != "" {
		candidate = filepath.Join(downloadsDir, fmt.Sprintf("%s_%s%s", base, short, ext))
		if !pathExists(candidate) {
			return candidate
		}
	}

	for i := 2; ; i++ {
		candidate = filepath.Join(downloadsDir, fmt.Sprintf("%s_%d%s", base, i, ext))
		if !pathExists(candidate) {
			return candidate
		}
	}
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func shortID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "-", "")
	if len(s) > 12 {
		s = s[:12]
	}
	return s
}

func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	name = invalidFilenameChars.ReplaceAllString(name, "_")
	name = strings.TrimSpace(name)
	name = strings.Join(strings.Fields(name), " ")
	name = strings.Trim(name, " ._")
	if name == "" {
		return ""
	}

	switch strings.ToUpper(name) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		name = "_" + name
	}

	const maxRunes = 120
	runes := []rune(name)
	if len(runes) > maxRunes {
		name = string(runes[:maxRunes])
	}
	return name
}

func detectAudioExtFromFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	if n <= 0 {
		return ".mp3", nil
	}
	return detectAudioExtFromBytes(buf[:n]), nil
}

func detectAudioExtFromBytes(b []byte) string {
	if len(b) >= 12 && bytes.HasPrefix(b, []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WAVE")) {
		return ".wav"
	}
	if len(b) >= 3 && bytes.HasPrefix(b, []byte("ID3")) {
		return ".mp3"
	}
	if len(b) >= 2 && b[0] == 0xFF && (b[1]&0xE0) == 0xE0 {
		return ".mp3"
	}
	if len(b) >= 8 && bytes.Equal(b[4:8], []byte("ftyp")) {
		return ".m4a"
	}
	return ".mp3"
}

func fetchBytes(ctx context.Context, rawURL string) ([]byte, error) {
	if st, err := os.Stat(rawURL); err == nil && !st.IsDir() {
		return os.ReadFile(rawURL)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if shouldBypassProxyHost(parsed.Hostname()) {
		transport.Proxy = nil
	}

	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "notebook-podcast-automator/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch input: status %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 2<<20))
}

func shouldBypassProxyHost(hostname string) bool {
	h := strings.ToLower(strings.TrimSpace(hostname))
	if h == "" || h == "localhost" {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate()
	}
	return false
}

func downloadToFile(ctx context.Context, outPath string, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid download url: %w", err)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if shouldBypassProxyHost(parsed.Hostname()) {
		transport.Proxy = nil
	}
	client := &http.Client{
		Timeout:   5 * time.Minute,
		Transport: transport,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}
	req.Header.Set("User-Agent", "notebook-podcast-automator/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download audio: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("download audio: status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close output file: %w", err)
	}

	return nil
}

func decodeMaybeDataURLBase64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty base64 payload")
	}

	if strings.HasPrefix(strings.ToLower(s), "data:") {
		if idx := strings.Index(s, ","); idx >= 0 {
			meta := strings.ToLower(s[:idx])
			if strings.Contains(meta, ";base64") {
				s = strings.TrimSpace(s[idx+1:])
			}
		}
	}

	return base64.StdEncoding.DecodeString(s)
}

func isUnavailable(err error) bool {
	var apiErr *batchexecute.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode != nil {
		return apiErr.ErrorCode.Type == batchexecute.ErrorTypeUnavailable
	}
	return false
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
