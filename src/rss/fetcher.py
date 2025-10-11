import requests
import feedparser
from typing import List, Optional
from datetime import datetime
import logging

from ..models.rss_item import RSSItem
from ..utils.logger import get_logger

logger = get_logger(__name__)


class RSSFetcher:
    """RSS订阅源获取器"""

    def __init__(self, user_agent: str = None, timeout: int = 30):
        self.user_agent = user_agent or "RSS-Pusher/1.0"
        self.timeout = timeout
        self.session = requests.Session()
        self.session.headers.update({
            'User-Agent': self.user_agent
        })

    def fetch_raw(self, url: str) -> Optional[str]:
        """获取RSS XML内容"""
        try:
            logger.info(f"正在获取RSS源: {url}")
            response = self.session.get(url, timeout=self.timeout)
            response.raise_for_status()
            logger.info(f"成功获取RSS内容，长度: {len(response.text)}")
            return response.text
        except requests.RequestException as e:
            logger.error(f"获取RSS源失败: {e}")
            return None
        except Exception as e:
            logger.error(f"获取RSS内容时发生异常: {e}")
            return None

    def fetch_parsed(self, url: str) -> Optional[feedparser.FeedParserDict]:
        """获取并解析RSS源"""
        raw_content = self.fetch_raw(url)
        if raw_content is None:
            return None

        try:
            # 使用feedparser解析RSS
            logger.info("开始解析RSS内容")
            feed = feedparser.parse(raw_content)

            if feed.bozo:
                # XML解析错误
                logger.warning(f"RSS解析警告: {feed.bozo_exception}")

            if not feed.entries:
                logger.warning("RSS源中没有找到任何条目")
                return None

            logger.info(f"成功解析RSS，找到 {len(feed.entries)} 个条目")
            return feed
        except Exception as e:
            logger.error(f"解析RSS时发生异常: {e}")
            return None

    def get_today_items(self, url: str, timezone_offset: int = 8) -> List[RSSItem]:
        """获取今天的RSS条目"""
        feed = self.fetch_parsed(url)
        if feed is None:
            return []

        items = []
        total_count = len(feed.entries)
        logger.info(f"从RSS源获取到 {total_count} 个条目，开始筛选今日内容")

        for entry in feed.entries:
            try:
                item = RSSItem.from_feedparser_entry(entry)

                # 只保留今天的内容
                if item.is_today(timezone_offset):
                    items.append(item)
                    logger.debug(f"找到今日条目: {item.title}")

            except Exception as e:
                logger.error(f"处理RSS条目时发生错误: {e}")
                continue

        logger.info(f"筛选出 {len(items)} 条今日内容")
        return items

    def get_feed_info(self, url: str) -> dict:
        """获取RSS源的基本信息"""
        feed = self.fetch_parsed(url)
        if feed is None:
            return {}

        info = {
            'title': feed.feed.get('title', ''),
            'description': feed.feed.get('description', ''),
            'link': feed.feed.get('link', ''),
            'updated': None,
            'generator': feed.feed.get('generator', ''),
            'language': feed.feed.get('language', ''),
            'total_entries': len(feed.entries)
        }

        # 解析更新时间
        if hasattr(feed.feed, 'updated_parsed') and feed.feed.updated_parsed:
            info['updated'] = datetime(*feed.feed.updated_parsed[:6])

        return info

    def validate_url(self, url: str) -> bool:
        """验证RSS源URL是否有效"""
        try:
            response = self.session.head(url, timeout=10)
            return response.status_code == 200
        except:
            return False

    def close(self):
        """关闭网络连接"""
        if self.session:
            self.session.close()