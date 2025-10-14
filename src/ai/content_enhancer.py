"""
内容增强器 - 使用两阶段筛选对RSS内容进行深度分析
阶段1: 基于标题+摘要快速打分筛选
阶段2: 获取全文后深度分析
"""

import os
import json
import re
from pathlib import Path
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

        # API统一配置(两个阶段共用)
        self.api_key = os.getenv('AI_API_KEY', '')
        self.api_base = os.getenv('AI_API_BASE', '')

        # 模型配置(两个阶段使用不同模型)
        self.stage1_model = os.getenv('STAGE1_MODEL', 'gpt-3.5-turbo')
        self.stage2_model = os.getenv('STAGE2_MODEL', 'gpt-4')

        # API提供商
        self.api_provider = config.get('provider', 'openai')

        # 内容抓取器
        self.content_fetcher = ContentFetcher() if self.enable_full_text else None

        # 加载系统提示词 (两个阶段分别加载)
        self.stage1_prompt = self._load_stage_prompt(1) if self.enable_two_stage else None
        self.stage2_prompt = self._load_stage_prompt(2)

        if self.enabled:
            if self.enable_two_stage:
                logger.info(f"两阶段筛选已启用 - 阶段1: {self.stage1_model}, 阶段2: {self.stage2_model}, 阈值: {self.stage1_threshold}分")
            else:
                logger.info(f"单阶段分析 - 模型: {self.stage2_model}")
        elif self.enabled and not self.api_key:
            logger.warning("内容增强器已启用但未配置API密钥")

    def _load_stage_prompt(self, stage: int) -> str:
        """加载指定阶段的系统提示词"""
        prompt_file = Path(__file__).parent.parent.parent / f'阶段{stage}提示词.md'

        if prompt_file.exists():
            try:
                with open(prompt_file, 'r', encoding='utf-8') as f:
                    content = f.read().strip()
                    logger.info(f"已加载阶段{stage}提示词: {prompt_file}")
                    return content
            except Exception as e:
                logger.error(f"加载阶段{stage}提示词失败: {e}")

        logger.warning(f"未找到阶段{stage}提示词文件，使用默认提示词")
        return self._get_default_prompt(stage)

    def _get_default_prompt(self, stage: int = 2) -> str:
        """默认提示词（简化版）"""
        if stage == 1:
            return "你是教育领域专家，帮助筛选与深圳教师社招考试主观题备考相关的文章。"
        else:
            return """你是一名教育内容分析专家。请分析RSS文章并按重要性分类：
- 核心必读 (★★★★★): 紧扣宏观教育政策、前沿理念
- 重点阅读 (★★★★☆): 实用的教学方法、案例
- 拓展阅读 (★★★☆☆): 理论知识、名家观点

请用Markdown表格格式输出，包含：文章标题、权重、推荐理由。"""

    def enhance_content(self, items: List[RSSItem]) -> Optional[str]:
        """
        对RSS内容进行增强分析(支持两阶段筛选)
        :param items: RSS条目列表
        :return: 增强后的Markdown格式内容
        """
        if not self.enabled:
            logger.info("内容增强器未启用")
            return None

        if not self.api_key:
            logger.error("内容增强器已启用但未配置API密钥")
            return None

        if not items:
            logger.warning("没有内容需要增强")
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

        # === 阶段1: 初筛 ===
        logger.info("阶段1: 基于标题+摘要进行快速打分...")
        scores = self._stage1_scoring(items)

        if not scores:
            logger.error("阶段1打分失败")
            return None

        # 筛选相关文章 (score=10表示相关, score=0表示不相关)
        high_score_items = []
        for item, score in zip(items, scores):
            if score >= self.stage1_threshold:  # threshold应设为7以下,确保10分可通过
                high_score_items.append((item, score))
                logger.info(f"✓ 通过初筛: {item.title}")
            else:
                logger.debug(f"✗ 未通过初筛: {item.title}")

        if not high_score_items:
            logger.warning(f"没有文章达到阈值 {self.stage1_threshold} 分")
            return None

        logger.info(f"阶段1完成: {len(high_score_items)}/{len(items)} 篇文章通过初筛")

        # === 获取全文 ===
        if self.enable_full_text and self.content_fetcher:
            logger.info("正在获取全文内容...")
            items_with_content = []
            for item, score in high_score_items:
                full_text = self.content_fetcher.fetch_content(item.link)
                if full_text:
                    # 创建带全文的新item副本
                    item_with_content = item
                    item_with_content.full_content = full_text
                    items_with_content.append((item_with_content, score))
                else:
                    logger.warning(f"无法获取全文: {item.title}")
                    items_with_content.append((item, score))  # 仍然使用原内容
            high_score_items = items_with_content

        # === 阶段2: 深度分析 ===
        logger.info(f"阶段2: 对 {len(high_score_items)} 篇文章进行深度分析...")
        filtered_items = [item for item, score in high_score_items]
        return self._stage2_deep_analysis(filtered_items)

    def _single_stage_enhance(self, items: List[RSSItem]) -> Optional[str]:
        """单阶段增强流程(直接深度分析)"""
        logger.info(f"开始单阶段分析: {len(items)} 个RSS条目")
        return self._stage2_deep_analysis(items)

    def _stage1_scoring(self, items: List[RSSItem]) -> Optional[List[int]]:
        """
        阶段1: 使用系统提示词对标题+摘要进行初筛
        :return: 相关性判断列表(1=相关需深度分析, 0=不相关可忽略)
        """
        # 构建文章列表
        rss_summary = "以下是RSS订阅内容:\n\n"

        for i, item in enumerate(items, 1):
            rss_summary += f"【文章{i}】\n"
            rss_summary += f"标题: {item.title}\n"
            description = item.get_excerpt(300)
            if description:
                rss_summary += f"摘要: {description}\n"
            rss_summary += "\n"

        # 构建阶段1筛选提示 - 简化版，不使用系统提示词
        stage1_prompt = f"""{rss_summary}

请判断每篇文章是否值得深入阅读。

**必须严格按照以下JSON格式返回，不要添加任何额外文字：**

{{
  "relevant": [1, 0, 1, 0, ...],
  "reason": ["相关", "不相关", ...]
}}

说明:
- relevant数组: 1=相关, 0=不相关
- reason数组: 简短说明(10字以内)
- 数组长度必须等于{len(items)}
"""

        # 调用阶段1 API (使用阶段1提示词)
        try:
            response = self._call_ai_with_config(
                prompt=stage1_prompt,
                api_key=self.api_key,
                api_base=self.api_base,
                model=self.stage1_model,
                system_prompt=self.stage1_prompt  # 使用阶段1提示词
            )

            if not response:
                return None

            # 解析相关性判断
            data = self._parse_json_response(response)
            if data and 'relevant' in data:
                relevant = data['relevant']
                reasons = data.get('reason', [''] * len(relevant))

                if len(relevant) == len(items):
                    # 转换为分数格式(1->10分通过, 0->0分不通过)
                    scores = [10 if r == 1 else 0 for r in relevant]

                    # 记录判断原因
                    for i, (item, r, reason) in enumerate(zip(items, relevant, reasons)):
                        if r == 1:
                            logger.debug(f"✓ 相关 [{reason}]: {item.title}")
                        else:
                            logger.debug(f"✗ 不相关 [{reason}]: {item.title}")

                    return scores
                else:
                    logger.error(f"返回的判断数量({len(relevant)})与文章数量({len(items)})不匹配")

            return None

        except Exception as e:
            logger.error(f"阶段1筛选失败: {e}")
            return None

    def _stage2_deep_analysis(self, items: List[RSSItem]) -> Optional[str]:
        """
        阶段2: 深度分析(使用系统提示词)
        :param items: 要分析的RSS条目(可能包含全文)
        :return: 格式化的Markdown内容
        """
        # 构建RSS内容摘要
        rss_summary = self._build_rss_summary(items, include_full_text=True)

        # 调用阶段2 API (使用阶段2提示词)
        ai_response = self._call_ai_with_config(
            prompt=rss_summary,
            api_key=self.api_key,
            api_base=self.api_base,
            model=self.stage2_model,
            system_prompt=self.stage2_prompt  # 使用阶段2提示词
        )

        if not ai_response:
            logger.error("阶段2分析失败")
            return None

        # 解析JSON
        analysis_data = self._parse_json_response(ai_response)

        if not analysis_data:
            logger.error("JSON解析失败")
            return None

        # 格式化输出
        formatted_content = self._format_beautiful_markdown(analysis_data, items)

        if formatted_content:
            logger.info("阶段2分析完成")
            return formatted_content
        else:
            logger.error("内容格式化失败")
            return None

    def _build_rss_summary(self, items: List[RSSItem], include_full_text: bool = False) -> str:
        """
        构建RSS内容摘要
        :param items: RSS条目列表
        :param include_full_text: 是否包含全文(如果有)
        """
        lines = ["以下是RSS订阅内容：\n"]

        for i, item in enumerate(items, 1):
            lines.append(f"【文章{i}】")
            lines.append(f"标题: {item.title}")
            lines.append(f"链接: {item.link}")

            # 如果有全文,使用全文;否则使用摘要
            if include_full_text and hasattr(item, 'full_content') and item.full_content:
                # 限制全文长度(避免超过token限制)
                full_text = item.full_content[:3000]
                lines.append(f"全文内容: {full_text}")
            else:
                description = item.get_excerpt(300)
                if description:
                    lines.append(f"描述: {description}")

            lines.append("")  # 空行分隔

        return "\n".join(lines)

    def _call_ai_with_config(
        self,
        prompt: str,
        api_key: str,
        api_base: str,
        model: str,
        system_prompt: Optional[str] = None
    ) -> Optional[str]:
        """
        使用指定配置调用AI API
        :param prompt: 用户提示词
        :param api_key: API密钥
        :param api_base: API基础URL
        :param model: 模型名称
        :param system_prompt: 系统提示词(可选)
        """
        try:
            if self.api_provider == 'openai':
                return self._call_openai_api(prompt, api_key, api_base, model, system_prompt)
            elif self.api_provider == 'claude':
                return self._call_claude_api(prompt, api_key, api_base, model, system_prompt)
            else:
                logger.error(f"不支持的API提供商: {self.api_provider}")
                return None
        except Exception as e:
            logger.error(f"调用AI API失败: {e}")
            return None

    def _call_openai_api(
        self,
        prompt: str,
        api_key: str,
        api_base: str,
        model: str,
        system_prompt: Optional[str] = None
    ) -> Optional[str]:
        """使用OpenAI兼容API"""
        try:
            from openai import OpenAI  # type: ignore
        except ImportError:
            logger.error("openai库未安装，请运行: pip install openai")
            return None

        try:
            # 创建客户端
            client = OpenAI(
                api_key=api_key,
                base_url=api_base if api_base else None
            )

            # 构建消息
            messages = []
            if system_prompt:
                messages.append({"role": "system", "content": system_prompt})
            messages.append({"role": "user", "content": prompt})

            # 调用API (Gemini有1M上下文,不限制max_tokens)
            response = client.chat.completions.create(
                model=model,
                messages=messages,
                temperature=0.3
                # 不设置max_tokens,让模型自由生成
            )

            # 调试：打印完整响应
            logger.debug(f"API响应对象: {response}")

            content = response.choices[0].message.content

            if not content:
                logger.error("AI返回的content为空")
                logger.error(f"完整响应: {response}")
                logger.error(f"choices: {response.choices}")
                if response.choices:
                    logger.error(f"第一个choice: {response.choices[0]}")
                    logger.error(f"message: {response.choices[0].message}")
                return None

            content = content.strip()
            logger.info(f"AI返回内容长度: {len(content)} 字符")

            return content

        except Exception as e:
            logger.error(f"OpenAI API调用失败: {e}")
            return None

    def _call_claude_api(
        self,
        prompt: str,
        api_key: str,
        api_base: str,
        model: str,
        system_prompt: Optional[str] = None
    ) -> Optional[str]:
        """使用Claude API"""
        try:
            import anthropic  # type: ignore
        except ImportError:
            logger.error("anthropic库未安装，请运行: pip install anthropic")
            return None

        try:
            client = anthropic.Anthropic(api_key=api_key)

            # 构建完整提示
            full_prompt = f"{system_prompt}\n\n{prompt}" if system_prompt else prompt

            # 调用API
            message = client.messages.create(
                model=model or "claude-3-haiku-20240307",
                max_tokens=2000,
                temperature=0.3,
                messages=[
                    {"role": "user", "content": full_prompt}
                ]
            )

            content = message.content[0].text.strip()
            logger.debug(f"AI返回内容长度: {len(content)} 字符")

            return content

        except Exception as e:
            logger.error(f"Claude API调用失败: {e}")
            return None

    def _parse_json_response(self, response: str) -> Optional[Dict]:
        """
        解析AI返回的JSON响应
        :param response: AI返回的文本
        :return: 解析后的字典，失败返回None
        """
        try:
            # 尝试直接解析
            return json.loads(response)
        except json.JSONDecodeError:
            # 如果失败，尝试提取JSON部分
            logger.warning("JSON解析失败，尝试提取JSON内容")
            logger.debug(f"原始响应内容（前500字符）: {response[:500]}")

            # 尝试提取 ```json ... ``` 中的内容
            json_match = re.search(r'```json\s*(.*?)\s*```', response, re.DOTALL)
            if json_match:
                try:
                    return json.loads(json_match.group(1))
                except json.JSONDecodeError:
                    pass

            # 尝试提取 { ... } 括号内容
            json_match = re.search(r'\{.*\}', response, re.DOTALL)
            if json_match:
                try:
                    return json.loads(json_match.group(0))
                except json.JSONDecodeError:
                    pass

            logger.error(f"无法从响应中提取有效JSON")
            logger.error(f"响应内容（前1000字符）: {response[:1000]}")
            return None

    def _format_beautiful_markdown(self, data: Dict, items: List[RSSItem]) -> str:
        """
        将AI分析的JSON数据格式化为美观的Markdown
        :param data: AI分析的JSON数据
        :param items: 原始RSS条目列表
        :return: 美化的Markdown字符串
        """
        lines = []

        # 标题和总结
        lines.append("# 📚 深圳教师社招主观热点素材\n")
        lines.append(f"**{data.get('summary', '为您筛选出高价值文章')}**\n")
        lines.append("---\n")

        # 遍历分类
        categories = data.get('categories', [])
        for category in categories:
            articles = category.get('articles', [])
            if not articles:
                continue

            # 分类标题
            name = category.get('name', '未分类')
            icon = category.get('icon', '📌')
            level = category.get('level', 3)
            stars = '★' * level + '☆' * (5 - level)
            description = category.get('description', '')

            lines.append(f"## {icon} {name} ({stars})\n")
            lines.append(f"*{description}*\n")

            # 文章列表
            for article in articles:
                article_id = article.get('article_id', 0)
                reason = article.get('reason', '推荐阅读')
                tags = article.get('tags', [])

                # 通过article_id匹配原始RSS条目
                if 1 <= article_id <= len(items):
                    item = items[article_id - 1]

                    # 文章条目（带链接和标签）
                    lines.append(f"### [{item.title}]({item.link})")
                    lines.append(f"💡 **推荐理由**：{reason}\n")

                    # 标签
                    if tags:
                        tag_str = ' '.join([f'`{tag}`' for tag in tags])
                        lines.append(f"🏷️ {tag_str}\n")

                    # 发布时间
                    if item.pub_date:
                        lines.append(f"📅 {item.pub_date.strftime('%Y-%m-%d %H:%M')}\n")

                    lines.append("")  # 空行分隔
                else:
                    logger.warning(f"article_id {article_id} 超出范围")

            lines.append("---\n")

        # 页脚
        lines.append("\n💡 **提示**：点击文章标题即可跳转阅读原文\n")
        lines.append("📌 **作者微信**：Xinx--1996\n")

        return "\n".join(lines)

    def __del__(self):
        """析构函数,清理资源"""
        if self.content_fetcher:
            self.content_fetcher.close()

    def __repr__(self) -> str:
        status = "启用" if self.enabled else "禁用"
        mode = "两阶段" if self.enable_two_stage else "单阶段"
        return f"ContentEnhancer(status={status}, mode={mode}, provider={self.api_provider})"
