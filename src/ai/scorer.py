"""
AI内容评分器
使用AI模型对RSS文章进行相关度评分和筛选
"""

import os
from pathlib import Path
from typing import List, Dict, Tuple
from dotenv import load_dotenv
from ..models.rss_item import RSSItem
from ..utils.logger import get_logger

# 加载环境变量
load_dotenv()

logger = get_logger(__name__)


class AIContentScorer:
    """AI内容评分器"""

    def __init__(self, config: Dict):
        """
        初始化AI评分器
        :param config: AI配置
        """
        self.enabled = config.get('enabled', False)
        self.api_provider = config.get('provider', 'openai')  # openai, claude, custom

        # 优先从环境变量读取API配置,其次从config读取
        self.api_key = os.getenv('AI_API_KEY') or config.get('api_key', '')
        self.api_base = os.getenv('AI_API_BASE') or config.get('api_base', '')
        self.model = os.getenv('AI_MODEL') or config.get('model', 'gpt-3.5-turbo')

        # 加载自定义提示词
        self.custom_prompt = self._load_custom_prompt()

        # 用户兴趣配置
        self.user_interests = config.get('interests', [])
        self.keywords_include = config.get('keywords_include', [])
        self.keywords_exclude = config.get('keywords_exclude', [])

        # 评分阈值
        self.min_score = config.get('min_score', 6.0)  # 最低6分才推送
        self.max_items = config.get('max_items', 15)  # 最多保留15条

        # 如果配置了API,记录日志
        if self.api_key:
            logger.info(f"AI API已配置: {self.api_base} (模型: {self.model})")

    def score_items(self, items: List[RSSItem]) -> List[Tuple[RSSItem, float]]:
        """
        对RSS条目进行AI评分
        :param items: RSS条目列表
        :return: (RSSItem, score)列表,按分数降序排列
        """
        if not self.enabled:
            logger.info("AI评分未启用,跳过评分")
            # 返回所有条目,分数为10
            return [(item, 10.0) for item in items]

        logger.info(f"开始AI评分,共 {len(items)} 个条目")

        scored_items = []
        for item in items:
            try:
                score = self._score_single_item(item)
                scored_items.append((item, score))
                logger.debug(f"评分: {score:.1f} - {item.title}")
            except Exception as e:
                logger.error(f"评分失败: {item.title} - {e}")
                # 失败的给默认分数5.0
                scored_items.append((item, 5.0))

        # 按分数降序排列
        scored_items.sort(key=lambda x: x[1], reverse=True)

        # 记录评分结果
        logger.info(f"评分完成,平均分: {sum(s for _, s in scored_items) / len(scored_items):.1f}")

        return scored_items

    def filter_and_rank(self, items: List[RSSItem]) -> List[RSSItem]:
        """
        筛选并排序RSS条目
        :param items: RSS条目列表
        :return: 筛选后的条目列表(按分数降序)
        """
        if not self.enabled:
            logger.info("AI筛选未启用,返回原始列表")
            return items

        # 1. AI评分
        scored_items = self.score_items(items)

        # 2. 过滤低分条目
        filtered_items = [
            (item, score) for item, score in scored_items
            if score >= self.min_score
        ]

        logger.info(f"筛选结果: {len(items)} -> {len(filtered_items)} 条 (阈值: {self.min_score}分)")

        # 3. 限制数量
        if len(filtered_items) > self.max_items:
            logger.info(f"限制数量: {len(filtered_items)} -> {self.max_items} 条")
            filtered_items = filtered_items[:self.max_items]

        # 4. 显示评分详情
        for i, (item, score) in enumerate(filtered_items[:10], 1):
            logger.info(f"  [{i}] {score:.1f}分 - {item.title}")

        # 返回条目列表(不含分数)
        return [item for item, _ in filtered_items]

    def _score_single_item(self, item: RSSItem) -> float:
        """
        对单个RSS条目评分
        :param item: RSS条目
        :return: 分数(0-10)
        """
        # 如果配置了API,调用AI模型评分
        if self.api_key:
            return self._ai_score(item)
        else:
            # 否则使用关键词规则评分
            return self._rule_based_score(item)

    def _ai_score(self, item: RSSItem) -> float:
        """
        使用AI模型评分
        :param item: RSS条目
        :return: 分数(0-10)
        """
        try:
            if self.api_provider == 'openai':
                return self._openai_score(item)
            elif self.api_provider == 'claude':
                return self._claude_score(item)
            else:
                # 默认使用规则评分
                return self._rule_based_score(item)
        except Exception as e:
            logger.error(f"AI评分失败: {e}")
            return self._rule_based_score(item)

    def _openai_score(self, item: RSSItem) -> float:
        """使用OpenAI兼容API评分(支持自定义API)"""
        try:
            try:
                import openai  # type: ignore
            except ImportError:
                logger.error("openai库未安装,请运行: pip install openai")
                return self._rule_based_score(item)

            # 配置API
            if self.api_base:
                openai.api_base = self.api_base
            openai.api_key = self.api_key

            # 构建提示词
            prompt = self._build_scoring_prompt(item)

            # 调用API
            response = openai.ChatCompletion.create(
                model=self.model,
                messages=[
                    {"role": "user", "content": prompt}
                ],
                temperature=0.3,
                max_tokens=150
            )

            # 解析分数
            content = response.choices[0].message.content.strip()
            score = self._parse_score(content)

            logger.debug(f"AI评分: {score:.1f} - {item.title[:30]}... | 响应: {content[:50]}")

            return score

        except Exception as e:
            logger.error(f"AI API调用失败: {e}")
            return self._rule_based_score(item)

    def _claude_score(self, item: RSSItem) -> float:
        """使用Claude API评分"""
        try:
            try:
                import anthropic  # type: ignore
            except ImportError:
                logger.error("anthropic库未安装,请运行: pip install anthropic")
                return self._rule_based_score(item)

            client = anthropic.Anthropic(api_key=self.api_key)

            # 构建提示词
            prompt = self._build_scoring_prompt(item)

            # 调用API
            message = client.messages.create(
                model=self.model or "claude-3-haiku-20240307",
                max_tokens=100,
                temperature=0.3,
                messages=[
                    {"role": "user", "content": prompt}
                ]
            )

            # 解析分数
            content = message.content[0].text.strip()
            score = self._parse_score(content)

            return score

        except Exception as e:
            logger.error(f"Claude API调用失败: {e}")
            return self._rule_based_score(item)

    def _rule_based_score(self, item: RSSItem) -> float:
        """
        基于关键词规则的评分(不需要API)
        :param item: RSS条目
        :return: 分数(0-10)
        """
        score = 5.0  # 基础分数

        text = (item.title + " " + item.description).lower()

        # 加分项:包含感兴趣的关键词
        for keyword in self.keywords_include:
            if keyword.lower() in text:
                score += 1.0
                logger.debug(f"匹配关键词 '{keyword}': +1.0")

        # 减分项:包含排除的关键词
        for keyword in self.keywords_exclude:
            if keyword.lower() in text:
                score -= 2.0
                logger.debug(f"匹配排除词 '{keyword}': -2.0")

        # 限制分数范围
        score = max(0.0, min(10.0, score))

        return score

    def _load_custom_prompt(self) -> str:
        """
        加载自定义提示词文件
        :return: 提示词内容
        """
        prompt_file = Path(__file__).parent.parent.parent / 'prompts' / 'scoring_prompt.txt'

        if prompt_file.exists():
            try:
                with open(prompt_file, 'r', encoding='utf-8') as f:
                    content = f.read().strip()
                    logger.info(f"已加载自定义提示词: {prompt_file}")
                    return content
            except Exception as e:
                logger.warning(f"加载提示词文件失败: {e}")

        return ""  # 返回空字符串,使用默认提示词

    def _build_scoring_prompt(self, item: RSSItem) -> str:
        """构建评分提示词"""
        # 如果有自定义提示词,使用自定义提示词
        if self.custom_prompt:
            system_prompt = self.custom_prompt
        else:
            # 使用默认提示词
            system_prompt = """你是一个专业的内容评分助手,负责根据用户的兴趣偏好对RSS文章进行评分。

评分标准:
- 9-10分: 非常相关,强烈推荐阅读
- 7-8分: 相关性较高,值得阅读
- 5-6分: 一般相关,可以浏览
- 3-4分: 相关度较低
- 0-2分: 不相关或无价值

请直接给出0-10之间的分数(可以是小数),并用一句话说明理由。
格式: 分数: X.X | 理由: XXX"""

        # 构建完整提示词
        prompt = f"""{system_prompt}

用户兴趣领域:
{', '.join(self.user_interests) if self.user_interests else '教育、科技、管理'}

文章标题: {item.title}
文章摘要: {item.get_excerpt(200)}
"""
        return prompt

    def _parse_score(self, content: str) -> float:
        """
        从AI响应中解析分数
        :param content: AI响应内容
        :return: 分数(0-10)
        """
        import re

        # 尝试匹配 "分数: X.X" 或 "Score: X.X" 格式
        match = re.search(r'(?:分数|score)[:：]\s*(\d+(?:\.\d+)?)', content, re.IGNORECASE)
        if match:
            score = float(match.group(1))
            return max(0.0, min(10.0, score))

        # 尝试匹配任何数字
        match = re.search(r'(\d+(?:\.\d+)?)', content)
        if match:
            score = float(match.group(1))
            return max(0.0, min(10.0, score))

        # 无法解析,返回默认分数
        logger.warning(f"无法解析分数: {content}")
        return 5.0

    def __repr__(self) -> str:
        status = "启用" if self.enabled else "禁用"
        return f"AIContentScorer(status={status}, provider={self.api_provider}, min_score={self.min_score})"
