from abc import ABC, abstractmethod
from typing import List, Dict, Any, Optional
from ..models.rss_item import RSSItem

class BasePusher(ABC):
    """推送接口抽象基类"""

    def __init__(self, config: Dict[str, Any], name: str = ""):
        self.config = config
        self.name = name or self.__class__.__name__
        self.enabled = config.get('enabled', True)

    @abstractmethod
    def initialize(self) -> bool:
        """
        初始化推送器
        :return: 初始化是否成功
        """
        pass

    @abstractmethod
    def push_items(self, items: List[RSSItem]) -> Dict[str, Any]:
        """
        推送RSS条目
        :param items: 要推送的RSS条目列表
        :return: 推送结果，包含成功/失败信息
        """
        pass

    @abstractmethod
    def validate_config(self) -> bool:
        """
        验证配置是否有效
        :return: 配置是否有效
        """
        pass

    def is_available(self) -> bool:
        """
        检查推送器是否可用
        :return: 是否可用
        """
        return self.enabled and self.validate_config()

    def format_message(self, items: List[RSSItem], template_config: Dict[str, Any] = None) -> str:
        """
        格式化推送消息
        :param items: RSS条目列表
        :param template_config: 消息模板配置
        :return: 格式化后的消息(仅包含标题+链接+时间,不再附带摘要和图片)
        """
        template_config = template_config or {}
        max_items = template_config.get('max_items', 0)

        if not items:
            return "今日暂无更新内容"

        # 限制条目数量(0表示不限制)
        if max_items > 0:
            items = items[:max_items]

        message_parts = [f"📰 今日新闻推送 ({len(items)}条)\n"]

        for i, item in enumerate(items, 1):
            # 统一仅推送标题 + 链接 + 时间, 摘要和首图由下游(如AI增强)统一处理
            message_parts.append(f"\n{i}. {item.title}")
            message_parts.append(f"   🔗 {item.link}")
            if item.pub_date:
                message_parts.append(f"   📅 {item.pub_date.strftime('%Y-%m-%d %H:%M:%S')}")

        message_parts.append("\n---\n📅 欢迎订阅RSS推送服务")

        return "\n".join(message_parts)

    def test_connection(self) -> Dict[str, Any]:
        """
        测试连接
        :return: 测试结果
        """
        try:
            # 默认的测试连接实现
            return {
                'success': True,
                'message': f"{self.name} 连接测试成功",
                'details': {}
            }
        except Exception as e:
            return {
                'success': False,
                'message': f"{self.name} 连接测试失败: {str(e)}",
                'details': {}
            }

    def get_push_statistics(self) -> Dict[str, Any]:
        """
        获取推送统计信息
        :return: 统计信息
        """
        # 默认实现，子类可以重写
        return {
            'total_pushes': 0,
            'successful_pushes': 0,
            'failed_pushes': 0,
            'last_push_time': None
        }

    def __str__(self) -> str:
        return f"{self.name}({'enabled' if self.enabled else 'disabled'})"

    def __repr__(self) -> str:
        return self.__str__()
