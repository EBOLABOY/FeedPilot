import yaml
import os
from pathlib import Path
from typing import Dict, Any, Optional
from ..utils.logger import get_logger

logger = get_logger(__name__)


class ConfigLoader:
    """配置文件加载器"""

    def __init__(self, config_file: str = "config/app.yaml"):
        self.config_file = Path(config_file)
        self._config: Optional[Dict[str, Any]] = None

    def load(self) -> Dict[str, Any]:
        """
        加载配置文件
        :return: 配置字典
        """
        if not self.config_file.exists():
            logger.error(f"配置文件不存在: {self.config_file}")
            raise FileNotFoundError(f"配置文件不存在: {self.config_file}")

        try:
            with open(self.config_file, 'r', encoding='utf-8') as f:
                self._config = yaml.safe_load(f)

            logger.info(f"成功加载配置文件: {self.config_file}")
            return self._config

        except yaml.YAMLError as e:
            logger.error(f"YAML解析错误: {e}")
            raise ValueError(f"配置文件格式错误: {e}")
        except Exception as e:
            logger.error(f"加载配置文件失败: {e}")
            raise

    def get(self, key: str, default: Any = None) -> Any:
        """
        获取配置项
        :param key: 配置键,支持点分隔的多级键(如 "rss.url")
        :param default: 默认值
        :return: 配置值
        """
        if self._config is None:
            self.load()

        keys = key.split('.')
        value = self._config

        for k in keys:
            if isinstance(value, dict):
                value = value.get(k)
                if value is None:
                    return default
            else:
                return default

        return value

    def get_rss_config(self) -> Dict[str, Any]:
        """获取RSS配置"""
        return self.get('rss', {})

    def get_push_config(self) -> Dict[str, Any]:
        """获取推送配置"""
        return self.get('push', {})

    def get_pushplus_config(self) -> Dict[str, Any]:
        """获取PushPlus配置"""
        return self.get('pushplus', {})

    def get_logging_config(self) -> Dict[str, Any]:
        """获取日志配置"""
        return self.get('logging', {})

    def get_database_config(self) -> Dict[str, Any]:
        """获取数据库配置"""
        return self.get('database', {})

    def get_scheduler_config(self) -> Dict[str, Any]:
        """获取调度器配置,支持从环境变量读取推送时间"""
        config = self.get('scheduler', {})

        # 如果环境变量中设置了推送时间,优先使用环境变量
        env_push_time = os.getenv('DAILY_PUSH_TIME')
        if env_push_time:
            config['daily_time'] = env_push_time
            logger.info(f"从环境变量读取推送时间: {env_push_time}")

        return config

    def get_enabled_pushers(self) -> list:
        """获取启用的推送器列表"""
        return self.get('push.enabled_pushers', [])

    def is_time_window_enabled(self) -> bool:
        """是否启用了推送时间窗口"""
        return self.get('push.time_window.enabled', False)

    def get_time_window(self) -> Dict[str, str]:
        """获取推送时间窗口"""
        return {
            'start': self.get('push.time_window.start', '00:00'),
            'end': self.get('push.time_window.end', '23:59')
        }

    def validate(self) -> bool:
        """
        验证配置的有效性
        :return: 是否有效
        """
        try:
            if self._config is None:
                self.load()

            # 验证必需的配置项
            required_keys = [
                'rss.url',
                'push.enabled_pushers',
                'logging.level',
                'database.type'
            ]

            for key in required_keys:
                value = self.get(key)
                if value is None:
                    logger.error(f"缺少必需的配置项: {key}")
                    return False

            # 验证RSS URL
            rss_url = self.get('rss.url')
            if not rss_url or not isinstance(rss_url, str):
                logger.error("RSS URL配置无效")
                return False

            # 验证启用的推送器
            enabled_pushers = self.get_enabled_pushers()
            if not enabled_pushers:
                logger.error("未配置任何推送器")
                return False

            # 验证每个启用的推送器配置
            for pusher in enabled_pushers:
                pusher_config = self.get(pusher, {})
                if not pusher_config:
                    logger.error(f"推送器 {pusher} 的配置不存在")
                    return False

                # 验证PushPlus配置
                if pusher == 'pushplus':
                    if not pusher_config.get('token'):
                        logger.error("PushPlus token未配置")
                        return False
                    if not pusher_config.get('topic'):
                        logger.error("PushPlus topic未配置")
                        return False

            logger.info("配置验证通过")
            return True

        except Exception as e:
            logger.error(f"配置验证失败: {e}")
            return False

    def reload(self) -> Dict[str, Any]:
        """重新加载配置文件"""
        logger.info("重新加载配置文件")
        self._config = None
        return self.load()

    def __repr__(self) -> str:
        return f"ConfigLoader(config_file='{self.config_file}')"


# 全局配置实例
_global_config: Optional[ConfigLoader] = None


def get_config(config_file: str = "config/app.yaml") -> ConfigLoader:
    """
    获取全局配置实例
    :param config_file: 配置文件路径
    :return: 配置加载器实例
    """
    global _global_config

    if _global_config is None:
        _global_config = ConfigLoader(config_file)
        _global_config.load()

    return _global_config


def load_config(config_file: str = "config/app.yaml") -> Dict[str, Any]:
    """
    加载配置文件的便捷函数
    :param config_file: 配置文件路径
    :return: 配置字典
    """
    loader = ConfigLoader(config_file)
    return loader.load()
