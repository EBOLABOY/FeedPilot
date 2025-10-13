"""
网页内容抓取模块
用于获取RSS链接的完整网页内容
"""

import requests
from bs4 import BeautifulSoup
import logging
from typing import Optional
import time

logger = logging.getLogger(__name__)


class ContentFetcher:
    """网页内容抓取器"""

    def __init__(self, timeout: int = 10, max_retries: int = 3):
        """
        初始化抓取器

        Args:
            timeout: 请求超时时间(秒)
            max_retries: 最大重试次数
        """
        self.timeout = timeout
        self.max_retries = max_retries
        self.session = requests.Session()
        self.session.headers.update({
            'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36'
        })

    def fetch_content(self, url: str) -> Optional[str]:
        """
        获取网页的文本内容

        Args:
            url: 网页URL

        Returns:
            提取的文本内容,如果失败返回None
        """
        for attempt in range(self.max_retries):
            try:
                logger.info(f"正在获取网页内容: {url} (尝试 {attempt + 1}/{self.max_retries})")

                response = self.session.get(url, timeout=self.timeout)
                response.raise_for_status()

                # 解析HTML
                soup = BeautifulSoup(response.content, 'html.parser')

                # 尝试找到主要内容区域
                main_content = self._extract_main_content(soup)

                if not main_content:
                    logger.warning(f"无法解析网页结构: {url}")
                    return None

                # 清理无关元素
                self._remove_noise_elements(main_content)

                # 提取文本
                text = main_content.get_text(separator='\n', strip=True)

                # 清理文本
                cleaned_text = self._clean_text(text)

                logger.info(f"成功获取内容,长度: {len(cleaned_text)} 字符")
                return cleaned_text

            except requests.exceptions.Timeout:
                logger.warning(f"请求超时: {url}")
                if attempt < self.max_retries - 1:
                    time.sleep(2 ** attempt)  # 指数退避

            except requests.exceptions.RequestException as e:
                logger.error(f"请求失败: {url}, 错误: {e}")
                if attempt < self.max_retries - 1:
                    time.sleep(2 ** attempt)

            except Exception as e:
                logger.error(f"解析网页内容失败: {url}, 错误: {e}")
                return None

        logger.error(f"获取网页内容失败,已达最大重试次数: {url}")
        return None

    def _extract_main_content(self, soup: BeautifulSoup) -> Optional[BeautifulSoup]:
        """
        智能提取主要内容区域

        Args:
            soup: BeautifulSoup对象

        Returns:
            主要内容区域的BeautifulSoup对象
        """
        # 优先级1: 微信公众号特定选择器
        wechat_selectors = [
            {'id': 'js_content'},  # 微信公众号正文容器
            {'class_': 'rich_media_content'},
        ]

        for selector in wechat_selectors:
            content = soup.find(**selector)
            if content:
                logger.debug(f"找到微信公众号内容: {selector}")
                return content

        # 优先级2: 常见的文章内容容器
        article_selectors = [
            {'name': 'article'},
            {'class_': 'article-content'},
            {'class_': 'post-content'},
            {'class_': 'entry-content'},
            {'id': 'article-content'},
        ]

        for selector in article_selectors:
            content = soup.find(**selector)
            if content:
                logger.debug(f"找到文章内容容器: {selector}")
                return content

        # 优先级3: 通用内容容器
        general_selectors = [
            {'name': 'main'},
            {'class_': 'content'},
            {'class_': 'main-content'},
            {'id': 'content'},
            {'id': 'main-content'},
        ]

        for selector in general_selectors:
            content = soup.find(**selector)
            if content:
                logger.debug(f"找到通用内容容器: {selector}")
                return content

        # 最后: 使用body
        logger.debug("使用body作为内容容器")
        return soup.find('body')

    def _remove_noise_elements(self, content: BeautifulSoup) -> None:
        """
        移除噪音元素(导航、页脚、侧边栏、广告等)

        Args:
            content: 内容区域的BeautifulSoup对象
        """
        # 移除标签类型
        noise_tags = [
            'script', 'style', 'iframe', 'noscript',  # 脚本和样式
            'nav', 'header', 'footer', 'aside',  # 页面结构
            'form', 'button',  # 交互元素
        ]

        for tag in noise_tags:
            for element in content.find_all(tag):
                element.decompose()

        # 移除特定class/id的噪音元素
        noise_patterns = [
            # 导航和菜单
            {'class_': lambda x: x and any(k in ' '.join(x) for k in ['nav', 'menu', 'sidebar', 'side-bar'])},
            # 页脚和版权信息
            {'class_': lambda x: x and any(k in ' '.join(x) for k in ['footer', 'copyright', 'license'])},
            # 广告和推广
            {'class_': lambda x: x and any(k in ' '.join(x) for k in ['ad', 'advertisement', 'promo', 'sponsor'])},
            # 评论和分享
            {'class_': lambda x: x and any(k in ' '.join(x) for k in ['comment', 'share', 'social', 'related'])},
            # 微信公众号特定噪音
            {'class_': lambda x: x and any(k in ' '.join(x) for k in ['qr-code', 'profile', 'recommend', 'tips'])},
            {'id': lambda x: x and any(k in x for k in ['qr', 'profile', 'recommend'])},
        ]

        for pattern in noise_patterns:
            for element in content.find_all(**pattern):
                element.decompose()

        # 移除常见的微信公众号底部提示文字
        for element in content.find_all(string=lambda text: text and any(
            keyword in text for keyword in [
                '微信扫一扫', '关注该公众号', '继续滑动', '轻触阅读原文',
                '预览时标签不可点', '知道了', '取消', '允许', '分享', '留言', '收藏',
                '赞', '在看', '视频', '小程序', '使用小程序', '提示：请点击',
            ]
        )):
            parent = element.parent
            if parent:
                parent.decompose()

    def _clean_text(self, text: str) -> str:
        """
        清理文本内容

        Args:
            text: 原始文本

        Returns:
            清理后的文本
        """
        # 按行分割
        lines = text.split('\n')

        # 清理每一行
        cleaned_lines = []
        for line in lines:
            line = line.strip()

            # 跳过空行
            if not line:
                continue

            # 跳过过短的行(可能是噪音)
            if len(line) < 3:
                continue

            # 跳过纯标点或特殊字符的行
            if all(not c.isalnum() for c in line):
                continue

            # 跳过常见噪音文本
            noise_keywords = [
                '微信扫', '关注', '继续滑动', '轻触阅读', '预览时',
                '知道了', '取消', '允许', '×', '分析', '使用完整服务',
                '，，', '。。', '、、',  # 重复标点
            ]

            if any(keyword in line for keyword in noise_keywords):
                continue

            cleaned_lines.append(line)

        # 合并并返回
        return '\n'.join(cleaned_lines)

    def close(self):
        """关闭会话"""
        self.session.close()
