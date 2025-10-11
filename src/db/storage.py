import sqlite3
from pathlib import Path
from typing import List, Optional, Set
from datetime import datetime
from ..models.rss_item import RSSItem
from ..utils.logger import get_logger

logger = get_logger(__name__)


class PushStorage:
    """推送记录存储管理器"""

    def __init__(self, db_path: str = "data/pushed_items.db"):
        self.db_path = Path(db_path)
        self.db_path.parent.mkdir(parents=True, exist_ok=True)
        self._connection: Optional[sqlite3.Connection] = None
        self._init_database()

    def _get_connection(self) -> sqlite3.Connection:
        """获取数据库连接"""
        if self._connection is None:
            self._connection = sqlite3.connect(self.db_path)
            self._connection.row_factory = sqlite3.Row
        return self._connection

    def _init_database(self):
        """初始化数据库表结构"""
        try:
            conn = self._get_connection()
            cursor = conn.cursor()

            # 创建推送记录表
            cursor.execute('''
                CREATE TABLE IF NOT EXISTS pushed_items (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    guid TEXT NOT NULL UNIQUE,
                    title TEXT NOT NULL,
                    link TEXT NOT NULL,
                    pushed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                    pusher_name TEXT,
                    success INTEGER DEFAULT 1
                )
            ''')

            # 创建索引
            cursor.execute('''
                CREATE INDEX IF NOT EXISTS idx_guid ON pushed_items(guid)
            ''')

            cursor.execute('''
                CREATE INDEX IF NOT EXISTS idx_pushed_at ON pushed_items(pushed_at)
            ''')

            conn.commit()
            logger.info(f"数据库初始化成功: {self.db_path}")

        except sqlite3.Error as e:
            logger.error(f"数据库初始化失败: {e}")
            raise

    def is_pushed(self, guid: str) -> bool:
        """
        检查某个条目是否已推送
        :param guid: RSS条目的唯一标识
        :return: 是否已推送
        """
        try:
            conn = self._get_connection()
            cursor = conn.cursor()

            cursor.execute(
                'SELECT COUNT(*) FROM pushed_items WHERE guid = ? AND success = 1',
                (guid,)
            )

            count = cursor.fetchone()[0]
            return count > 0

        except sqlite3.Error as e:
            logger.error(f"查询推送记录失败: {e}")
            return False

    def mark_as_pushed(self, item: RSSItem, pusher_name: str = "", success: bool = True) -> bool:
        """
        标记条目为已推送
        :param item: RSS条目
        :param pusher_name: 推送器名称
        :param success: 是否推送成功
        :return: 是否标记成功
        """
        try:
            conn = self._get_connection()
            cursor = conn.cursor()

            cursor.execute('''
                INSERT OR REPLACE INTO pushed_items
                (guid, title, link, pusher_name, success, pushed_at)
                VALUES (?, ?, ?, ?, ?, ?)
            ''', (
                item.guid,
                item.title,
                item.link,
                pusher_name,
                1 if success else 0,
                datetime.now()
            ))

            conn.commit()
            logger.debug(f"标记条目为已推送: {item.title}")
            return True

        except sqlite3.Error as e:
            logger.error(f"标记推送记录失败: {e}")
            return False

    def mark_items_as_pushed(self, items: List[RSSItem], pusher_name: str = "", success: bool = True) -> int:
        """
        批量标记条目为已推送
        :param items: RSS条目列表
        :param pusher_name: 推送器名称
        :param success: 是否推送成功
        :return: 成功标记的数量
        """
        success_count = 0

        try:
            conn = self._get_connection()
            cursor = conn.cursor()

            pushed_at = datetime.now()
            success_flag = 1 if success else 0

            data = [
                (item.guid, item.title, item.link, pusher_name, success_flag, pushed_at)
                for item in items
            ]

            cursor.executemany('''
                INSERT OR REPLACE INTO pushed_items
                (guid, title, link, pusher_name, success, pushed_at)
                VALUES (?, ?, ?, ?, ?, ?)
            ''', data)

            conn.commit()
            success_count = len(items)
            logger.info(f"批量标记 {success_count} 个条目为已推送")

        except sqlite3.Error as e:
            logger.error(f"批量标记推送记录失败: {e}")

        return success_count

    def filter_unpushed_items(self, items: List[RSSItem]) -> List[RSSItem]:
        """
        过滤出未推送的条目
        :param items: RSS条目列表
        :return: 未推送的条目列表
        """
        if not items:
            return []

        unpushed_items = []

        for item in items:
            if not self.is_pushed(item.guid):
                unpushed_items.append(item)

        logger.info(f"从 {len(items)} 个条目中筛选出 {len(unpushed_items)} 个未推送条目")
        return unpushed_items

    def get_pushed_guids(self, days: int = 7) -> Set[str]:
        """
        获取最近N天已推送的GUID集合
        :param days: 天数
        :return: GUID集合
        """
        try:
            conn = self._get_connection()
            cursor = conn.cursor()

            cursor.execute('''
                SELECT guid FROM pushed_items
                WHERE pushed_at >= datetime('now', ? || ' days')
                AND success = 1
            ''', (f'-{days}',))

            guids = {row[0] for row in cursor.fetchall()}
            logger.debug(f"获取到最近{days}天的{len(guids)}个已推送GUID")
            return guids

        except sqlite3.Error as e:
            logger.error(f"获取已推送GUID集合失败: {e}")
            return set()

    def get_push_history(self, limit: int = 100) -> List[dict]:
        """
        获取推送历史记录
        :param limit: 返回记录数量
        :return: 推送记录列表
        """
        try:
            conn = self._get_connection()
            cursor = conn.cursor()

            cursor.execute('''
                SELECT id, guid, title, link, pushed_at, pusher_name, success
                FROM pushed_items
                ORDER BY pushed_at DESC
                LIMIT ?
            ''', (limit,))

            records = []
            for row in cursor.fetchall():
                records.append({
                    'id': row['id'],
                    'guid': row['guid'],
                    'title': row['title'],
                    'link': row['link'],
                    'pushed_at': row['pushed_at'],
                    'pusher_name': row['pusher_name'],
                    'success': bool(row['success'])
                })

            return records

        except sqlite3.Error as e:
            logger.error(f"获取推送历史失败: {e}")
            return []

    def get_statistics(self) -> dict:
        """
        获取推送统计信息
        :return: 统计信息字典
        """
        try:
            conn = self._get_connection()
            cursor = conn.cursor()

            # 总推送数
            cursor.execute('SELECT COUNT(*) FROM pushed_items')
            total_count = cursor.fetchone()[0]

            # 成功推送数
            cursor.execute('SELECT COUNT(*) FROM pushed_items WHERE success = 1')
            success_count = cursor.fetchone()[0]

            # 失败推送数
            failed_count = total_count - success_count

            # 今日推送数
            cursor.execute('''
                SELECT COUNT(*) FROM pushed_items
                WHERE DATE(pushed_at) = DATE('now')
            ''')
            today_count = cursor.fetchone()[0]

            # 本周推送数
            cursor.execute('''
                SELECT COUNT(*) FROM pushed_items
                WHERE pushed_at >= datetime('now', '-7 days')
            ''')
            week_count = cursor.fetchone()[0]

            # 最后推送时间
            cursor.execute('''
                SELECT pushed_at FROM pushed_items
                ORDER BY pushed_at DESC LIMIT 1
            ''')
            last_row = cursor.fetchone()
            last_pushed = last_row[0] if last_row else None

            return {
                'total_count': total_count,
                'success_count': success_count,
                'failed_count': failed_count,
                'today_count': today_count,
                'week_count': week_count,
                'last_pushed': last_pushed
            }

        except sqlite3.Error as e:
            logger.error(f"获取推送统计失败: {e}")
            return {}

    def cleanup_old_records(self, days: int = 30) -> int:
        """
        清理旧的推送记录
        :param days: 保留最近N天的记录
        :return: 删除的记录数
        """
        try:
            conn = self._get_connection()
            cursor = conn.cursor()

            cursor.execute('''
                DELETE FROM pushed_items
                WHERE pushed_at < datetime('now', ? || ' days')
            ''', (f'-{days}',))

            deleted_count = cursor.rowcount
            conn.commit()

            logger.info(f"清理了 {deleted_count} 条超过{days}天的旧记录")
            return deleted_count

        except sqlite3.Error as e:
            logger.error(f"清理旧记录失败: {e}")
            return 0

    def close(self):
        """关闭数据库连接"""
        if self._connection:
            self._connection.close()
            self._connection = None
            logger.debug("数据库连接已关闭")

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        self.close()

    def __repr__(self) -> str:
        return f"PushStorage(db_path='{self.db_path}')"
