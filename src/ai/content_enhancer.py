"""
内容增强器 - 使用专业提示词对RSS内容进行深度分析和分类
用于深圳市小学教师招聘考试备考内容筛选
"""

import os
import json
import re
from pathlib import Path
from typing import List, Dict, Optional
from dotenv import load_dotenv
from ..models.rss_item import RSSItem
from ..utils.logger import get_logger

# 加载环境变量
load_dotenv()

logger = get_logger(__name__)


class ContentEnhancer:
    """内容增强器 - 对RSS内容进行深度分析和结构化"""

    def __init__(self, config: Dict):
        """
        初始化内容增强器
        :param config: 增强器配置
        """
        self.enabled = config.get('enabled', False)
        self.api_provider = config.get('provider', 'openai')

        # API配置
        self.api_key = os.getenv('AI_API_KEY') or config.get('api_key', '')
        self.api_base = os.getenv('AI_API_BASE') or config.get('api_base', '')
        self.model = os.getenv('AI_MODEL') or config.get('model', 'gpt-3.5-turbo')

        # 加载系统提示词
        self.system_prompt = self._load_system_prompt()

        if self.enabled and self.api_key:
            logger.info(f"内容增强器已启用: {self.api_provider} ({self.model})")
        elif self.enabled:
            logger.warning("内容增强器已启用但未配置API密钥，将无法工作")

    def _load_system_prompt(self) -> str:
        """加载系统提示词"""
        prompt_file = Path(__file__).parent.parent.parent / '系统提示词.md'

        if prompt_file.exists():
            try:
                with open(prompt_file, 'r', encoding='utf-8') as f:
                    content = f.read().strip()
                    logger.info(f"已加载系统提示词: {prompt_file}")
                    return content
            except Exception as e:
                logger.error(f"加载系统提示词失败: {e}")

        logger.warning("未找到系统提示词文件，使用默认提示词")
        return self._get_default_prompt()

    def _get_default_prompt(self) -> str:
        """默认提示词（简化版）"""
        return """你是一名教育内容分析专家。请分析RSS文章并按重要性分类：
- 核心必读 (★★★★★): 紧扣宏观教育政策、前沿理念
- 重点阅读 (★★★★☆): 实用的教学方法、案例
- 拓展阅读 (★★★☆☆): 理论知识、名家观点

请用Markdown表格格式输出，包含：文章标题、权重、推荐理由。"""

    def enhance_content(self, items: List[RSSItem]) -> Optional[str]:
        """
        对RSS内容进行深度分析和增强
        :param items: RSS条目列表
        :return: 增强后的美化Markdown格式内容
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
            logger.info(f"开始增强 {len(items)} 个RSS条目的内容")

            # 构建RSS内容摘要
            rss_summary = self._build_rss_summary(items)

            # 调用AI进行分析，获取JSON
            ai_response = self._call_ai_api(rss_summary)

            if not ai_response:
                logger.error("AI未返回内容")
                return None

            # 解析JSON
            analysis_data = self._parse_json_response(ai_response)

            if not analysis_data:
                logger.error("JSON解析失败")
                return None

            # 美化输出为Markdown
            formatted_content = self._format_beautiful_markdown(analysis_data, items)

            if formatted_content:
                logger.info("内容增强完成")
                return formatted_content
            else:
                logger.error("内容格式化失败")
                return None

        except Exception as e:
            logger.error(f"内容增强过程出错: {e}", exc_info=True)
            return None

    def _build_rss_summary(self, items: List[RSSItem]) -> str:
        """构建RSS内容摘要（用于发送给AI）"""
        lines = ["以下是RSS订阅内容：\n"]

        for i, item in enumerate(items, 1):
            lines.append(f"【文章{i}】")
            lines.append(f"标题: {item.title}")
            lines.append(f"链接: {item.link}")

            # 获取描述（限制长度）
            description = item.get_excerpt(300)
            if description:
                lines.append(f"描述: {description}")

            lines.append("")  # 空行分隔

        return "\n".join(lines)

    def _call_ai_api(self, rss_summary: str) -> Optional[str]:
        """调用AI API进行内容分析"""
        try:
            if self.api_provider == 'openai':
                return self._call_openai_api(rss_summary)
            elif self.api_provider == 'claude':
                return self._call_claude_api(rss_summary)
            else:
                logger.error(f"不支持的API提供商: {self.api_provider}")
                return None
        except Exception as e:
            logger.error(f"调用AI API失败: {e}")
            return None

    def _call_openai_api(self, rss_summary: str) -> Optional[str]:
        """使用OpenAI兼容API"""
        try:
            from openai import OpenAI  # type: ignore
        except ImportError:
            logger.error("openai库未安装，请运行: pip install openai")
            return None

        try:
            # 创建客户端
            client = OpenAI(
                api_key=self.api_key,
                base_url=self.api_base if self.api_base else None
            )

            # 构建消息
            messages = [
                {"role": "system", "content": self.system_prompt},
                {"role": "user", "content": rss_summary}
            ]

            # 调用API
            response = client.chat.completions.create(
                model=self.model,
                messages=messages,
                temperature=0.3,
                max_tokens=8000  # 增加token以支持详细分析（gemini-2.5-pro需要更多token）
            )

            content = response.choices[0].message.content

            if not content:
                logger.error("AI返回的content为空")
                logger.error(f"完整响应: {response}")
                return None

            content = content.strip()
            logger.info(f"AI返回内容长度: {len(content)} 字符")
            logger.debug(f"AI返回内容前200字符: {content[:200]}")

            return content

        except Exception as e:
            logger.error(f"OpenAI API调用失败: {e}")
            return None

    def _call_claude_api(self, rss_summary: str) -> Optional[str]:
        """使用Claude API"""
        try:
            import anthropic  # type: ignore
        except ImportError:
            logger.error("anthropic库未安装，请运行: pip install anthropic")
            return None

        try:
            client = anthropic.Anthropic(api_key=self.api_key)

            # 构建完整提示（Claude需要将system prompt合并到用户消息中）
            full_prompt = f"{self.system_prompt}\n\n{rss_summary}"

            # 调用API
            message = client.messages.create(
                model=self.model or "claude-3-haiku-20240307",
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

            logger.error(f"无法从响应中提取有效JSON: {response[:200]}...")
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
        lines.append("# 📚 教师招聘考试备考推送\n")
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
        lines.append("📌 **来源**：RSS推送服务 | 自动分析推送\n")

        return "\n".join(lines)

    def __repr__(self) -> str:
        status = "启用" if self.enabled else "禁用"
        return f"ContentEnhancer(status={status}, provider={self.api_provider})"
