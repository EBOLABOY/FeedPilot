import logging
import os
from logging.handlers import RotatingFileHandler
from typing import Optional
from pathlib import Path


_logger_instances = {}


def get_logger(name: str = __name__) -> logging.Logger:
    """
    获取或创建logger实例
    :param name: logger名称,通常使用__name__
    :return: logger实例
    """
    if name in _logger_instances:
        return _logger_instances[name]

    logger = logging.getLogger(name)
    _logger_instances[name] = logger

    return logger


def setup_logging(
    log_file: Optional[str] = None,
    level: str = "INFO",
    max_bytes: int = 10 * 1024 * 1024,  # 10MB
    backup_count: int = 5,
    console_output: bool = True
):
    """
    配置全局日志系统
    :param log_file: 日志文件路径
    :param level: 日志级别 (DEBUG, INFO, WARNING, ERROR, CRITICAL)
    :param max_bytes: 单个日志文件最大大小(字节)
    :param backup_count: 保留的日志文件数量
    :param console_output: 是否输出到控制台
    """
    # 解析日志级别
    numeric_level = getattr(logging, level.upper(), logging.INFO)

    # 创建根logger
    root_logger = logging.getLogger()
    root_logger.setLevel(numeric_level)

    # 清除现有的handlers
    root_logger.handlers.clear()

    # 定义日志格式
    formatter = logging.Formatter(
        '%(asctime)s - %(name)s - %(levelname)s - %(message)s',
        datefmt='%Y-%m-%d %H:%M:%S'
    )

    # 控制台处理器
    if console_output:
        console_handler = logging.StreamHandler()
        console_handler.setLevel(numeric_level)
        console_handler.setFormatter(formatter)
        root_logger.addHandler(console_handler)

    # 文件处理器
    if log_file:
        # 确保日志目录存在
        log_path = Path(log_file)
        log_path.parent.mkdir(parents=True, exist_ok=True)

        file_handler = RotatingFileHandler(
            log_file,
            maxBytes=max_bytes,
            backupCount=backup_count,
            encoding='utf-8'
        )
        file_handler.setLevel(numeric_level)
        file_handler.setFormatter(formatter)
        root_logger.addHandler(file_handler)

    # 记录启动日志
    root_logger.info(f"日志系统初始化完成 - 级别: {level}, 文件: {log_file or '未配置'}")


def parse_size(size_str: str) -> int:
    """
    解析大小字符串(如 "10MB", "1GB")为字节数
    :param size_str: 大小字符串
    :return: 字节数
    """
    size_str = size_str.upper().strip()

    units = {
        'B': 1,
        'KB': 1024,
        'MB': 1024 * 1024,
        'GB': 1024 * 1024 * 1024,
    }

    for unit, multiplier in units.items():
        if size_str.endswith(unit):
            try:
                number = float(size_str[:-len(unit)])
                return int(number * multiplier)
            except ValueError:
                pass

    # 默认返回10MB
    return 10 * 1024 * 1024


def configure_from_dict(config: dict):
    """
    从配置字典配置日志系统
    :param config: 日志配置字典
    """
    log_file = config.get('file', 'logs/app.log')
    level = config.get('level', 'INFO')
    max_size = config.get('max_size', '10MB')
    backup_count = config.get('backup_count', 5)

    # 解析大小
    max_bytes = parse_size(max_size)

    setup_logging(
        log_file=log_file,
        level=level,
        max_bytes=max_bytes,
        backup_count=backup_count,
        console_output=True
    )


# 默认配置(如果没有调用setup_logging)
if not logging.getLogger().handlers:
    setup_logging(console_output=True, level="INFO")
