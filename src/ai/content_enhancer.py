import os
import json
import re
from pathlib import Path
from datetime import datetime
from typing import List, Dict, Optional, Tuple
from dotenv import load_dotenv
from ..models.rss_item import RSSItem
from ..utils.logger import get_logger
from ..utils.content_fetcher import ContentFetcher

# 加载环境变量
load_dotenv()

logger = get_logger(__name__)


class ContentEnhancer:
    """内容增强器 - 两阶段筛选和深度分析"""

    def __init__(self, config: Dict):
        """
        初始化内容增强器
        :param config: 增强器配置
        """
        self.enabled = config.get('enabled', False)

        # 两阶段配置
        self.enable_two_stage = os.getenv('ENABLE_TWO_STAGE', 'true').lower() == 'true'
        self.enable_full_text = os.getenv('ENABLE_FULL_TEXT', 'true').lower() == 'true'
        self.stage1_threshold = int(os.getenv('STAGE1_SCORE_THRESHOLD', '7'))

        # API统一配置
        self.api_key = os.getenv('AI_API_KEY', '')
        self.api_base = os.getenv('AI_API_BASE', '')

        # 模型配置
        self.stage1_model = os.getenv('STAGE1_MODEL', 'gpt-3.5-turbo')
        self.stage2_model = os.getenv('STAGE2_MODEL', 'gpt-4')

        # API提供商
        self.api_provider = config.get('provider', 'openai')

        # 内容抓取器
        self.content_fetcher = ContentFetcher() if self.enable_full_text else None

        # 加载系统提示词
        self.stage1_prompt = self._load_stage_prompt(1) if self.enable_two_stage else None
        self.stage2_prompt = self._load_stage_prompt(2)

        if self.enabled:
            logger.info(f"AI增强服务状态: 启用 | 双阶段: {self.enable_two_stage} | 全文分析: {self.enable_full_text}")
            if not self.api_key:
                logger.warning("警告: 未检测到 AI_API_KEY，增强功能可能无法工作")

    def _load_stage_prompt(self, stage: int) -> str:
        """加载指定阶段的系统提示词"""
        prompt_file = Path(__file__).parent.parent.parent / f'阶段{stage}提示词.md'

        if prompt_file.exists():
            try:
                with open(prompt_file, 'r', encoding='utf-8') as f:
                    content = f.read().strip()
                    logger.info(f"已加载阶段{stage}提示词")
                    return content
            except Exception as e:
                logger.error(f"加载阶段{stage}提示词失败: {e}")

        logger.warning(f"未找到阶段{stage}提示词文件，使用默认提示词")
        return self._get_default_prompt(stage)

    def _get_default_prompt(self, stage: int = 2) -> str:
        """默认提示词（简化版）"""
        return "你是教育资讯分析专家。"

    def enhance_content(self, items: List[RSSItem]) -> Optional[str]:
        """
        对RSS内容进行增强分析(支持两阶段筛选)
        :param items: RSS条目列表
        :return: 增强后的Markdown格式内容
        """
        if not self.enabled or not self.api_key or not items:
            return None

        try:
            if self.enable_two_stage:
                return self._two_stage_enhance(items)
            else:
                return self._single_stage_enhance(items)
        except Exception as e:
            logger.error(f"内容增强过程出错: {e}", exc_info=True)
            return None

    def _two_stage_enhance(self, items: List[RSSItem]) -> Optional[str]:
        """两阶段增强流程"""
        logger.info(f"开始两阶段分析: {len(items)} 个RSS条目")

        # === 阶段1: 初筛 (Broad Filter) ===
        scores = self._stage1_scoring(items)

        if not scores:
            logger.warning("阶段1打分未返回结果，跳过分析")
            return None

        # 筛选 (threshold=1, 即只要相关就保留)
        high_score_items = []
        for item, score in zip(items, scores):
            if score >= 1: 
                high_score_items.append((item, score))

        if not high_score_items:
            logger.warning("没有文章通过初筛")
            return None

        logger.info(f"阶段1完成: {len(high_score_items)}/{len(items)} 篇文章通过初筛")

        # === 阶段1.5: 获取全文 (Safe Mode) ===
        # 为了防止 Prompt 过长，我们限制每篇文章的全文长度，或者仅对前 N 篇获取全文
        filtered_items_objs = [item for item, score in high_score_items]
        
        if self.enable_full_text and self.content_fetcher:
            logger.info("正在获取全文内容...")
            items_with_content = []
            for item in filtered_items_objs:
                try:
                    full_text = self.content_fetcher.fetch_content(item.link)
                    if full_text:
                        item.full_content = full_text
                except Exception as e:
                    logger.warning(f"获取全文失败 {item.title}: {e}")
                items_with_content.append(item)
            filtered_items_objs = items_with_content

        # === 阶段2: 深度分析 ===
        return self._stage2_deep_analysis(filtered_items_objs)

    def _single_stage_enhance(self, items: List[RSSItem]) -> Optional[str]:
        """单阶段增强流程"""
        return self._stage2_deep_analysis(items)

    def _stage1_scoring(self, items: List[RSSItem]) -> Optional[List[int]]:
        """阶段1: 快速初筛"""
        # 构建轻量级 Prompt (仅标题)
        rss_brief = "\n".join([f"{i+1}. {item.title}" for i, item in enumerate(items)])
        
        prompt = f"""以下是RSS文章列表：
{rss_brief}

请根据【阶段1提示词】判断相关性。
只返回JSON: {{"relevant": [1, 0, ...], "reason": [...]}}
确保数组长度为 {len(items)}。
"""
        
        try:
            response = self._call_ai_with_config(
                prompt=prompt,
                api_key=self.api_key,
                api_base=self.api_base,
                model=self.stage1_model,
                system_prompt=self.stage1_prompt
            )

            if not response: 
                return None

            data = self._parse_json_response(response)
            if data and 'relevant' in data:
                return data['relevant']
            
            return None
        except Exception as e:
            logger.error(f"阶段1筛选异常: {e}")
            return None

    def _stage2_deep_analysis(self, items: List[RSSItem]) -> Optional[str]:
        """阶段2: 深度分析"""
        # 构建摘要，此处限制长度以防 Prompt 爆炸
        rss_summary = self._build_rss_summary(items, include_full_text=True, max_chars=800)

        ai_response = self._call_ai_with_config(
            prompt=rss_summary,
            api_key=self.api_key,
            api_base=self.api_base,
            model=self.stage2_model,
            system_prompt=self.stage2_prompt
        )

        if not ai_response:
            return None

        # 增强的 JSON 解析
        analysis_data = self._parse_json_response(ai_response)
        
        if not analysis_data:
            # 记录原始内容以便调试
            logger.error(f"JSON解析彻底失败。Raw Response Preview: {ai_response[:500]}...")
            return None

        return self._format_beautiful_markdown(analysis_data, items)

    def _build_rss_summary(self, items: List[RSSItem], include_full_text: bool = False, max_chars: int = 800) -> str:
        """构建RSS摘要，严格限制长度"""
        lines = ["以下是待分析的文章内容：\n"]

        for i, item in enumerate(items, 1):
            lines.append(f"【文章{i}】")
            lines.append(f"标题: {item.title}")
            lines.append(f"链接: {item.link}")

            content = ""
            if include_full_text and hasattr(item, 'full_content') and item.full_content:
                content = item.full_content[:max_chars]  # 严格截断
                lines.append(f"正文片段: {content}...")
            else:
                description = item.get_excerpt(300)
                if description:
                    lines.append(f"摘要: {description}")
            
            lines.append("")

        return "\n".join(lines)

    def _call_ai_with_config(self, prompt: str, api_key: str, api_base: str, model: str, system_prompt: Optional[str] = None) -> Optional[str]:
        """通用AI调用 (OpenAI/Claude)"""
        if self.api_provider == 'openai':
            return self._call_openai_api(prompt, api_key, api_base, model, system_prompt)
        elif self.api_provider == 'claude':
            return self._call_claude_api(prompt, api_key, api_base, model, system_prompt)
        return None

    def _call_openai_api(self, prompt: str, api_key: str, api_base: str, model: str, system_prompt: Optional[str] = None) -> Optional[str]:
        try:
            from openai import OpenAI
            client = OpenAI(api_key=api_key, base_url=api_base if api_base else None)
            
            messages = []
            if system_prompt:
                messages.append({"role": "system", "content": system_prompt})
            messages.append({"role": "user", "content": prompt})

            # 使用 json_object 模式 (如果模型支持)
            response = client.chat.completions.create(
                model=model,
                messages=messages,
                temperature=0.3,
                response_format={"type": "json_object"} if "gpt-4" in model or "gpt-3.5-turbo-1106" in model else None
            )
            
            return self._extract_openai_content(response)

        except Exception as e:
            logger.error(f"OpenAI API error: {e}")
            return None

    def _extract_openai_content(self, response) -> Optional[str]:
        """健壮的响应内容提取，兼容 Object/Dict/String"""
        try:
            # 1. Pydantic Object (OpenAI v1 Standard)
            if hasattr(response, "choices"):
                return response.choices[0].message.content
            
            # 2. Key/Value Access (Dict)
            if isinstance(response, dict):
                return response.get('choices')[0].get('message').get('content')

            # 3. String (Raw JSON or Direct Content)
            if isinstance(response, str):
                # 尝试当作 JSON 解析
                try:
                    data = json.loads(response)
                    if isinstance(data, dict) and 'choices' in data:
                        return data['choices'][0]['message']['content']
                except:
                    pass
                # 如果不是 JSON，假设就是内容本身（或者是错误信息）
                return response

            logger.error(f"无法识别的响应类型: {type(response)}")
            return None
        except Exception as e:
            logger.error(f"响应提取失败: {e}. Raw: {str(response)[:200]}...")
            return None

    def _call_claude_api(self, prompt: str, api_key: str, api_base: str, model: str, system_prompt: Optional[str] = None) -> Optional[str]:
        # Claude 实现... (省略以节省篇幅，保留原逻辑)
        # 这里假设用户主要用 OpenAI。如果用 Claude，逻辑类似。
        try:
            import anthropic
            client = anthropic.Anthropic(api_key=api_key)
            full_prompt = f"{system_prompt}\n\n{prompt}" if system_prompt else prompt
            message = client.messages.create(
                model=model or "claude-3-haiku-20240307",
                max_tokens=4000,
                temperature=0.3,
                messages=[{"role": "user", "content": full_prompt}]
            )
            return message.content[0].text
        except Exception as e:
            logger.error(f"Claude API error: {e}")
            return None

    def _parse_json_response(self, response: str) -> Optional[Dict]:
        """解析JSON，增加容错"""
        if not response: return None
        
        # 1. 尝试直接解析
        try:
            return json.loads(response)
        except:
            pass
            
        # 2. 清理 Markdown 标记
        clean_response = re.sub(r'^```json\s*', '', response.strip())
        clean_response = re.sub(r'\s*```$', '', clean_response)
        
        try:
            return json.loads(clean_response)
        except:
            pass
            
        # 3. 提取最外层 {} (处理包含前言后语的情况)
        json_match = re.search(r'\{(?:[^{}]|(?R))*\}', response, re.DOTALL) # 递归匹配太复杂，用简单贪婪匹配
        json_match = re.search(r'\{.*\}', response, re.DOTALL)
        if json_match:
            try:
                return json.loads(json_match.group(0))
            except:
                pass
        
        # 4. 终极尝试：dirtyjson (如果未安装则跳过)
        # 这里不引入新依赖，只是记录错误
        return None

    def _format_beautiful_markdown(self, data: Dict, items: List[RSSItem]) -> str:
        """生成 Markdown 报告"""
        lines = []
        lines.append("# 📚 育见-日报\n")
        lines.append(f"📅 {datetime.now().strftime('%Y-%m-%d')}\n")

        # Summary Section
        summary_section = data.get('summary_section')
        if summary_section and isinstance(summary_section, dict):
             title = summary_section.get('title', '今日核心洞察')
             insight = summary_section.get('insight', '')
             trends = summary_section.get('trends', [])
             
             lines.append(f"## 🧐 {title}\n")
             if insight:
                 lines.append(f"{insight}\n")
             
             if trends:
                 lines.append("\n**📉 关键趋势：**\n")
                 for trend in trends:
                     lines.append(f"- {trend}")
                 lines.append("\n")
             lines.append("---\n")
        
        # Categories
        categories = data.get('categories', [])
        for category in categories:
            articles = category.get('articles', [])
            if not articles: continue
            
            name = category.get('name', '板块')
            icon = category.get('icon', '📌')
            lines.append(f"## {icon} {name}\n")
            
            for article in articles:
                # 健壮性：支持 article_id 或者是直接包含 title/link 的对象
                item = None
                article_id = article.get('article_id')
                if article_id and isinstance(article_id, int) and 1 <= article_id <= len(items):
                    item = items[article_id - 1]
                
                # 如果找不到对应的 RSS 原始条目，则跳过
                if not item: continue
                
                reason = article.get('reason', '')
                lines.append(f"### [{item.title}]({item.link})")
                lines.append(f"💡 {reason}\n")
                lines.append("")
            
            lines.append("---\n")
            
        lines.append("\n📌 **育见·日报** | AI驱动的教育内参\n")
        
        return "\n".join(lines)
