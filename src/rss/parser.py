from typing import List
from ..models.rss_item import RSSItem
from ..utils.logger import get_logger

logger = get_logger(__name__)


class RSSParser:
    """RSS解析器，专门处理RSS条目的解析和过滤"""

    def __init__(self, timezone_offset: int = 8):
        """
        初始化RSS解析器
        :param timezone_offset: 时区偏移（小时），默认为8（UTC+8）
        """
        self.timezone_offset = timezone_offset

    def filter_today_items(self, items: List[RSSItem]) -> List[RSSItem]:
        """过滤出今天的RSS条目"""
        today_items = []

        for item in items:
            if item.is_today(self.timezone_offset):
                today_items.append(item)
                logger.debug(f"今日条目: {item.title}")
            else:
                logger.debug(f"非今日条目: {item.title} (发布时间: {item.pub_date})")

        logger.info(f"从 {len(items)} 个条目中筛选出 {len(today_items)} 个今日条目")
        return today_items

    def deduplicate_items(self, items: List[RSSItem]) -> List[RSSItem]:
        """去重RSS条目（根据guid）"""
        seen_guids = set()
        unique_items = []

        for item in items:
            if item.guid not in seen_guids:
                seen_guids.add(item.guid)
                unique_items.append(item)
            else:
                logger.debug(f"重复条目已跳过: {item.title}")

        logger.info(f"去重后剩余 {len(unique_items)} 个条目")
        return unique_items

    def sort_by_publish_time(self, items: List[RSSItem], reverse: bool = True) -> List[RSSItem]:
        """按发布时间排序RSS条目"""
        # 过滤出有发布时间的条目
        items_with_date = [item for item in items if item.pub_date is not None]
        items_without_date = [item for item in items if item.pub_date is None]

        # 有发布时间的按时间排序
        items_with_date.sort(key=lambda x: x.pub_date, reverse=reverse)

        # 没有发布时间的放在最后
        sorted_items = items_with_date + items_without_date

        logger.info(f"按发布时间排序完成，{len(items_with_date)} 个条目有时间，{len(items_without_date)} 个条目无时间")
        return sorted_items

    def limit_items(self, items: List[RSSItem], max_items: int) -> List[RSSItem]:
        """限制RSS条目数量"""
        if len(items) <= max_items:
            return items

        limited_items = items[:max_items]
        logger.info(f"限制RSS条目数量为 {max_items}，总共 {len(items)} 个条目")

        return limited_items

    def process_items(self, items: List[RSSItem], max_items: int = None) -> List[RSSItem]:
        """完整的RSS条目处理流程"""
        logger.info(f"开始处理 {len(items)} 个RSS条目")

        # 1. 过滤今日条目
        items = self.filter_today_items(items)

        # 2. 去重
        items = self.deduplicate_items(items)

        # 3. 按时间排序
        items = self.sort_by_publish_time(items)

        # 4. 限制数量
        if max_items and max_items > 0:
            items = self.limit_items(items, max_items)

        logger.info(f"处理完成，最终得到 {len(items)} 个条目")
        return items

    def extract_search_keywords(self, items: List[RSSItem]) -> List[str]:
        """从RSS条目中提取搜索关键词（可用于后续的标签分类等）"""
        keywords = []
        for item in items:
            # 简单的关键词提取：从标题中提取
            title_words = item.title.split()
            keywords.extend(title_words[:3])  # 取前3个词

        # 去重并返回
        unique_keywords = list(set(keywords))
        logger.info(f"提取到 {len(unique_keywords)} 个关键词")
        return unique_keywords