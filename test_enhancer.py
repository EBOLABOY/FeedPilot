#!/usr/bin/env python3
"""
测试内容增强器 - 验证链接功能
"""

import sys
from pathlib import Path

# 添加src目录到路径
sys.path.insert(0, str(Path(__file__).parent / 'src'))

from src.models.rss_item import RSSItem
from src.ai.content_enhancer import ContentEnhancer
from datetime import datetime

def create_test_items():
    """创建测试RSS条目"""
    items = [
        RSSItem(
            title="教育强国建设纲要发布",
            link="http://example.com/article1",
            description="教育部发布教育强国建设纲要，明确提出到2035年建成教育强国的目标...",
            pub_date=datetime.now(),
            guid="test1"
        ),
        RSSItem(
            title="AI赋能课堂教学创新实践",
            link="http://example.com/article2",
            description="人工智能技术与课堂教学深度融合，探索智能化教学新模式...",
            pub_date=datetime.now(),
            guid="test2"
        ),
        RSSItem(
            title="项目式学习(PBL)设计案例分享",
            link="http://example.com/article3",
            description="分享一个完整的项目式学习设计案例，包含教学目标、活动流程、评价标准...",
            pub_date=datetime.now(),
            guid="test3"
        ),
    ]
    return items

def test_link_generation():
    """测试链接生成功能"""
    print("="*60)
    print("测试内容增强器 - 链接功能")
    print("="*60)

    # 创建测试条目
    items = create_test_items()
    print(f"\n✓ 创建了 {len(items)} 个测试RSS条目")

    # 显示测试条目
    print("\n测试条目:")
    for i, item in enumerate(items, 1):
        print(f"  {i}. {item.title}")
        print(f"     链接: {item.link}")

    # 创建增强器实例（使用测试配置）
    config = {
        'enabled': True,
        'provider': 'openai',
    }

    enhancer = ContentEnhancer(config)
    print(f"\n✓ 内容增强器实例创建成功")

    # 测试RSS摘要生成（包含链接提示）
    rss_summary = enhancer._build_rss_summary(items)
    print("\n" + "="*60)
    print("生成的RSS摘要（发送给AI的内容）:")
    print("="*60)
    print(rss_summary)

    # 检查是否包含链接提示
    if "Markdown链接格式" in rss_summary and "[文章标题](链接地址)" in rss_summary:
        print("\n✅ 成功：RSS摘要中包含链接格式提示")
    else:
        print("\n❌ 失败：RSS摘要中缺少链接格式提示")

    print("\n" + "="*60)
    print("测试完成!")
    print("="*60)
    print("\n提示:")
    print("1. 系统会要求AI在输出中使用 [标题](URL) 格式")
    print("2. 实际测试需要配置 AI_API_KEY 并运行 python main.py --once")
    print("3. 推送到PushPlus后，点击标题即可跳转到原文")

if __name__ == '__main__':
    test_link_generation()
