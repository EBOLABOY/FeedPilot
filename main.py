#!/usr/bin/env python3
"""
RSS推送服务入口
"""
import sys
from pathlib import Path

# 添加src目录到路径
sys.path.insert(0, str(Path(__file__).parent / 'src'))

from src.cli import main

if __name__ == '__main__':
    main()
