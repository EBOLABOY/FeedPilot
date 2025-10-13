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

                # 移除脚本和样式标签
                for script in soup(["script", "style", "nav", "footer", "header"]):
                    script.decompose()

                # 尝试找到主要内容区域
                main_content = None

                # 常见的内容容器标签和类名
                content_selectors = [
                    {'name': 'article'},
                    {'name': 'main'},
                    {'class_': 'content'},
                    {'class_': 'article'},
                    {'class_': 'post'},
                    {'class_': 'entry-content'},
                    {'id': 'content'},
                    {'id': 'main-content'},
                ]

                for selector in content_selectors:
                    main_content = soup.find(**selector)
                    if main_content:
                        logger.debug(f"找到内容容器: {selector}")
                        break

                # 如果没找到特定容器,使用body
                if not main_content:
                    main_content = soup.find('body')

                if not main_content:
                    logger.warning(f"无法解析网页结构: {url}")
                    return None

                # 提取文本
                text = main_content.get_text(separator='\n', strip=True)

                # 清理多余的空行
                lines = [line.strip() for line in text.split('\n') if line.strip()]
                cleaned_text = '\n'.join(lines)

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

    def close(self):
        """关闭会话"""
        self.session.close()
