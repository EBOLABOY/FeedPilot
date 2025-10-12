import requests
from typing import List, Dict, Any
from .base import BasePusher
from ..models.rss_item import RSSItem
from ..utils.logger import get_logger

logger = get_logger(__name__)


class PushPlusPusher(BasePusher):
    """PushPlus推送器"""

    API_URL = "http://www.pushplus.plus/send"

    def __init__(self, config: Dict[str, Any]):
        super().__init__(config, name="PushPlus")
        self.token = config.get('token', '')
        self.topic = config.get('topic', '')
        self.template = config.get('message_template', {}).get('template', 'html')

    def initialize(self) -> bool:
        """初始化推送器"""
        try:
            if not self.validate_config():
                logger.error("PushPlus配置验证失败")
                return False

            logger.info(f"PushPlus推送器初始化成功, Topic: {self.topic}")
            return True
        except Exception as e:
            logger.error(f"PushPlus推送器初始化失败: {e}")
            return False

    def validate_config(self) -> bool:
        """验证配置是否有效"""
        if not self.token:
            logger.error("PushPlus token未配置")
            return False

        if not self.topic:
            logger.error("PushPlus topic(群组编号)未配置")
            return False

        return True

    def push_items(self, items: List[RSSItem]) -> Dict[str, Any]:
        """
        推送RSS条目到PushPlus群组
        :param items: 要推送的RSS条目列表
        :return: 推送结果
        """
        if not items:
            logger.warning("没有需要推送的内容")
            return {
                'success': False,
                'message': '没有需要推送的内容',
                'details': {}
            }

        try:
            # 格式化消息(不再限制数量,由调用方控制)
            if self.template == 'html':
                title, content = self._format_html_message(items)
            elif self.template == 'markdown':
                title, content = self._format_markdown_message(items)
            else:
                title, content = self._format_text_message(items)

            # 构建请求参数
            payload = {
                'token': self.token,
                'title': title,
                'content': content,
                'topic': self.topic,
                'template': self.template
            }

            logger.info(f"开始推送消息到PushPlus, 标题: {title}, 条目数: {len(items)}")

            # 发送请求
            response = requests.post(self.API_URL, json=payload, timeout=30)
            response.raise_for_status()

            result = response.json()

            if result.get('code') == 200:
                logger.info(f"PushPlus推送成功: {result.get('msg', '')}")
                return {
                    'success': True,
                    'message': 'PushPlus推送成功',
                    'details': {
                        'items_count': len(items),
                        'response': result
                    }
                }
            else:
                error_msg = result.get('msg', '未知错误')
                logger.error(f"PushPlus推送失败: {error_msg}")
                return {
                    'success': False,
                    'message': f'PushPlus推送失败: {error_msg}',
                    'details': result
                }

        except requests.RequestException as e:
            logger.error(f"PushPlus网络请求失败: {e}")
            return {
                'success': False,
                'message': f'网络请求失败: {str(e)}',
                'details': {}
            }
        except Exception as e:
            logger.error(f"PushPlus推送时发生异常: {e}")
            return {
                'success': False,
                'message': f'推送异常: {str(e)}',
                'details': {}
            }

    def _format_html_message(self, items: List[RSSItem]) -> tuple:
        """格式化HTML消息"""
        template_config = self.config.get('message_template', {})
        include_description = template_config.get('include_description', True)
        include_image = template_config.get('include_image', False)

        title = f"📰 今日RSS推送 ({len(items)}条)"

        html_parts = [
            '<html>',
            '<head><meta charset="utf-8"></head>',
            '<body style="font-family: Arial, sans-serif; line-height: 1.6;">',
            f'<h2 style="color: #333;">{title}</h2>',
            '<hr style="border: 1px solid #ddd;">',
        ]

        for i, item in enumerate(items, 1):
            html_parts.append(f'<div style="margin-bottom: 30px; padding: 15px; background: #f9f9f9; border-radius: 5px;">')
            html_parts.append(f'<h3 style="margin-top: 0; color: #2c3e50;">{i}. {item.title}</h3>')

            if include_description and item.get_excerpt():
                html_parts.append(f'<p style="color: #555; margin: 10px 0;">{item.get_excerpt(200)}</p>')

            html_parts.append(f'<p><a href="{item.link}" style="color: #3498db; text-decoration: none;">🔗 查看详情</a></p>')

            if include_image:
                image_url = item.extract_first_image()
                if image_url:
                    html_parts.append(f'<img src="{image_url}" alt="文章配图" style="max-width: 100%; height: auto; border-radius: 5px;">')

            if item.pub_date:
                html_parts.append(f'<p style="color: #999; font-size: 0.9em; margin-top: 10px;">📅 {item.pub_date.strftime("%Y-%m-%d %H:%M:%S")}</p>')

            html_parts.append('</div>')

        html_parts.extend([
            '<hr style="border: 1px solid #ddd; margin-top: 30px;">',
            '<p style="text-align: center; color: #999; font-size: 0.9em;">📬 RSS推送服务 | 自动推送</p>',
            '</body>',
            '</html>'
        ])

        return title, ''.join(html_parts)

    def _format_markdown_message(self, items: List[RSSItem]) -> tuple:
        """格式化Markdown消息"""
        template_config = self.config.get('message_template', {})
        include_description = template_config.get('include_description', True)
        include_image = template_config.get('include_image', False)

        title = f"📰 今日RSS推送 ({len(items)}条)"

        md_parts = [f"# {title}\n"]

        for i, item in enumerate(items, 1):
            md_parts.append(f"\n## {i}. {item.title}\n")

            if include_description and item.get_excerpt():
                md_parts.append(f"\n{item.get_excerpt(200)}\n")

            md_parts.append(f"\n[🔗 查看详情]({item.link})\n")

            if include_image:
                image_url = item.extract_first_image()
                if image_url:
                    md_parts.append(f"\n![文章配图]({image_url})\n")

            if item.pub_date:
                md_parts.append(f"\n📅 {item.pub_date.strftime('%Y-%m-%d %H:%M:%S')}\n")

            md_parts.append("\n---\n")

        md_parts.append("\n📬 RSS推送服务 | 自动推送\n")

        return title, ''.join(md_parts)

    def _format_text_message(self, items: List[RSSItem]) -> tuple:
        """格式化纯文本消息"""
        template_config = self.config.get('message_template', {})
        include_description = template_config.get('include_description', True)

        title = f"📰 今日RSS推送 ({len(items)}条)"

        text_parts = [title, "\n" + "="*50 + "\n"]

        for i, item in enumerate(items, 1):
            text_parts.append(f"\n{i}. {item.title}\n")

            if include_description and item.get_excerpt():
                text_parts.append(f"📝 {item.get_excerpt(150)}\n")

            text_parts.append(f"🔗 {item.link}\n")

            if item.pub_date:
                text_parts.append(f"📅 {item.pub_date.strftime('%Y-%m-%d %H:%M:%S')}\n")

            text_parts.append("-"*50 + "\n")

        text_parts.append("\n📬 RSS推送服务 | 自动推送\n")

        return title, ''.join(text_parts)

    def push_custom_message(self, title: str, content: str, template: str = 'markdown') -> Dict[str, Any]:
        """
        推送自定义消息（用于内容增强）
        :param title: 消息标题
        :param content: 消息内容
        :param template: 消息格式 (html/markdown/txt)
        :return: 推送结果
        """
        try:
            payload = {
                'token': self.token,
                'title': title,
                'content': content,
                'topic': self.topic,
                'template': template
            }

            logger.info(f"开始推送自定义消息到PushPlus, 标题: {title}")

            response = requests.post(self.API_URL, json=payload, timeout=30)
            response.raise_for_status()

            result = response.json()

            if result.get('code') == 200:
                logger.info(f"PushPlus推送成功: {result.get('msg', '')}")
                return {
                    'success': True,
                    'message': 'PushPlus推送成功',
                    'details': result
                }
            else:
                error_msg = result.get('msg', '未知错误')
                logger.error(f"PushPlus推送失败: {error_msg}")
                return {
                    'success': False,
                    'message': f'PushPlus推送失败: {error_msg}',
                    'details': result
                }

        except requests.RequestException as e:
            logger.error(f"PushPlus网络请求失败: {e}")
            return {
                'success': False,
                'message': f'网络请求失败: {str(e)}',
                'details': {}
            }
        except Exception as e:
            logger.error(f"PushPlus推送时发生异常: {e}")
            return {
                'success': False,
                'message': f'推送异常: {str(e)}',
                'details': {}
            }

    def test_connection(self) -> Dict[str, Any]:
        """测试PushPlus连接"""
        try:
            payload = {
                'token': self.token,
                'title': 'RSS推送服务 - 连接测试',
                'content': '这是一条测试消息,用于验证PushPlus配置是否正确。',
                'topic': self.topic,
                'template': 'txt'
            }

            response = requests.post(self.API_URL, json=payload, timeout=10)
            response.raise_for_status()
            result = response.json()

            if result.get('code') == 200:
                logger.info("PushPlus连接测试成功")
                return {
                    'success': True,
                    'message': 'PushPlus连接测试成功',
                    'details': result
                }
            else:
                error_msg = result.get('msg', '未知错误')
                logger.error(f"PushPlus连接测试失败: {error_msg}")
                return {
                    'success': False,
                    'message': f'连接测试失败: {error_msg}',
                    'details': result
                }

        except Exception as e:
            logger.error(f"PushPlus连接测试异常: {e}")
            return {
                'success': False,
                'message': f'连接测试异常: {str(e)}',
                'details': {}
            }

    def __str__(self) -> str:
        return f"PushPlusPusher(topic={self.topic}, enabled={self.enabled})"
