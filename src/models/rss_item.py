from urllib.parse import urlparse
from typing import Optional
from datetime import datetime, date

class RSSItem:
    """RSS新闻项数据模型"""

    def __init__(self, title: str, link: str, description: str = "",
                 pub_date: Optional[datetime] = None, guid: str = ""):
        self.title = title.strip()
        self.link = link.strip()
        self.description = description.strip()
        self.pub_date = pub_date
        self.guid = guid.strip() or link  # 如果没有guid，使用link作为唯一标识

    @classmethod
    def from_feedparser_entry(cls, entry) -> 'RSSItem':
        """从feedparser的entry创建RSSItem"""
        title = entry.get('title', '')
        link = entry.get('link', '')

        # 处理描述，优先使用summary，其次用description
        description = ''
        if hasattr(entry, 'summary') and entry.summary:
            description = entry.summary
        elif hasattr(entry, 'description') and entry.description:
            description = entry.description

        # 解析发布时间
        pub_date = None
        if hasattr(entry, 'published_parsed') and entry.published_parsed:
            try:
                pub_date = datetime(*entry.published_parsed[:6])
            except (TypeError, ValueError):
                pass

        if pub_date is None and hasattr(entry, 'updated_parsed') and entry.updated_parsed:
            try:
                pub_date = datetime(*entry.updated_parsed[:6])
            except (TypeError, ValueError):
                pass

        # 如果feedparser解析失败,尝试手动解析published/updated字段
        if pub_date is None:
            try:
                from dateutil import parser as date_parser
                for field in ['published', 'updated', 'pubDate']:
                    if hasattr(entry, field):
                        date_str = getattr(entry, field)
                        if date_str:
                            try:
                                # 手动解析日期字符串
                                pub_date = date_parser.parse(date_str, ignoretz=False)
                                break
                            except:
                                pass
            except ImportError:
                # 如果没有dateutil,忽略
                pass

        guid = entry.get('id', '')

        return cls(title, link, description, pub_date, guid)

    def is_today(self, timezone_offset_hours: int = 0, default_if_no_date: bool = True) -> bool:
        """
        判断是否为今天的新闻
        :param timezone_offset_hours: 时区偏移小时数
        :param default_if_no_date: 如果没有发布时间,默认返回值(True表示当作今日内容)
        :return: 是否为今日内容
        """
        if self.pub_date is None:
            # 如果没有发布时间,根据default_if_no_date参数决定
            return default_if_no_date

        # 考虑时区偏移
        if timezone_offset_hours != 0:
            from datetime import timedelta
            adjusted_date = self.pub_date + timedelta(hours=timezone_offset_hours)
            return adjusted_date.date() == date.today()

        return self.pub_date.date() == date.today()

    def get_excerpt(self, max_length: int = 100) -> str:
        """获取摘要（移除HTML标签并限制长度）"""
        import re
        # 移除HTML标签
        text = re.sub(r'<[^>]+>', '', self.description)
        # 移除多余的空白字符
        text = re.sub(r'\s+', ' ', text).strip()

        if len(text) <= max_length:
            return text
        return text[:max_length] + "..."

    def extract_first_image(self) -> Optional[str]:
        """从描述中提取第一张图片URL"""
        import re
        # 匹配img标签的src属性
        img_match = re.search(r'<img[^>]+src=["\']([^"\']+)["\']', self.description)
        if img_match:
            return img_match.group(1)
        return None

    def __str__(self) -> str:
        return f"RSSItem(title='{self.title}', link='{self.link}', pub_date={self.pub_date})"

    def __repr__(self) -> str:
        return self.__str__()

    def __eq__(self, other) -> bool:
        if not isinstance(other, RSSItem):
            return False
        return self.guid == other.guid

    def __hash__(self) -> int:
        return hash(self.guid)