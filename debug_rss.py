#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
调试脚本 - 查看RSS源的实际数据
"""

import sys
import io
from pathlib import Path
from datetime import datetime, date

# 设置stdout为UTF-8编码
sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8')

# 添加src目录到路径
sys.path.insert(0, str(Path(__file__).parent / 'src'))

from src.rss.fetcher import RSSFetcher
from src.models.rss_item import RSSItem

def main():
    rss_url = "http://47.79.39.147:8001/feed/all.rss"

    print("="*80)
    print("RSS源数据调试工具")
    print("="*80)
    print(f"RSS源地址: {rss_url}")
    print(f"当前日期: {date.today()}")
    print(f"当前时间: {datetime.now()}")
    print("="*80)

    # 获取RSS
    fetcher = RSSFetcher()
    feed = fetcher.fetch_parsed(rss_url)

    if not feed or not feed.entries:
        print("[ERROR] 无法获取RSS内容")
        return

    print(f"\n[OK] 成功获取 {len(feed.entries)} 个RSS条目\n")
    print("="*80)
    print("前10条RSS条目详情:")
    print("="*80)

    for i, entry in enumerate(feed.entries[:10], 1):
        try:
            item = RSSItem.from_feedparser_entry(entry)

            print(f"\n【条目 {i}】")
            print(f"标题: {item.title}")
            print(f"GUID: {item.guid}")
            print(f"发布时间: {item.pub_date}")

            if item.pub_date:
                print(f"发布日期: {item.pub_date.date()}")
                print(f"今天日期: {date.today()}")
                print(f"是否今日: {item.pub_date.date() == date.today()}")

                # 检查不同时区偏移
                from datetime import timedelta
                for offset in [0, 8, -8]:
                    adjusted_date = (item.pub_date + timedelta(hours=offset)).date()
                    is_today = adjusted_date == date.today()
                    print(f"  时区偏移 {offset:+3d}h: {adjusted_date} -> {'[今日]' if is_today else '[非今日]'}")
            else:
                print("[WARN] 无发布时间")

            # 显示原始时间数据
            if hasattr(entry, 'published_parsed') and entry.published_parsed:
                print(f"原始published_parsed: {entry.published_parsed}")
            if hasattr(entry, 'updated_parsed') and entry.updated_parsed:
                print(f"原始updated_parsed: {entry.updated_parsed}")

            print("-"*80)

        except Exception as e:
            print(f"[ERROR] 解析条目 {i} 失败: {e}")

    print("\n" + "="*80)
    print("统计信息:")
    print("="*80)

    # 统计今日条目
    today_count_no_offset = 0
    today_count_with_offset = 0

    for entry in feed.entries:
        try:
            item = RSSItem.from_feedparser_entry(entry)
            if item.pub_date:
                # 不带时区偏移
                if item.pub_date.date() == date.today():
                    today_count_no_offset += 1

                # 带+8时区偏移
                from datetime import timedelta
                adjusted_date = (item.pub_date + timedelta(hours=8)).date()
                if adjusted_date == date.today():
                    today_count_with_offset += 1
        except:
            pass

    print(f"总条目数: {len(feed.entries)}")
    print(f"今日条目(无时区偏移): {today_count_no_offset}")
    print(f"今日条目(+8小时偏移): {today_count_with_offset}")
    print("="*80)

if __name__ == '__main__':
    main()
