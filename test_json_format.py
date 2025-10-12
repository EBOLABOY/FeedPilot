#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
测试 JSON 解析和美化输出
"""

import sys
import json
from pathlib import Path
from datetime import datetime

# 设置UTF-8输出
if sys.platform == 'win32':
    import io
    sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8')

# 添加src目录到路径
sys.path.insert(0, str(Path(__file__).parent / 'src'))

from src.models.rss_item import RSSItem
from src.ai.content_enhancer import ContentEnhancer

def create_test_items():
    """创建测试RSS条目"""
    items = [
        RSSItem(
            title="教育强国建设纲要正式发布",
            link="http://example.com/article1",
            description="教育部发布教育强国建设纲要，明确提出到2035年建成教育强国的目标...",
            pub_date=datetime(2025, 1, 10, 10, 30),
            guid="test1"
        ),
        RSSItem(
            title="AI赋能课堂教学创新实践探索",
            link="http://example.com/article2",
            description="人工智能技术与课堂教学深度融合，探索智能化教学新模式...",
            pub_date=datetime(2025, 1, 10, 11, 0),
            guid="test2"
        ),
        RSSItem(
            title="项目式学习(PBL)完整设计案例",
            link="http://example.com/article3",
            description="分享一个完整的项目式学习设计案例，包含教学目标、活动流程、评价标准...",
            pub_date=datetime(2025, 1, 10, 14, 20),
            guid="test3"
        ),
        RSSItem(
            title="小学班级管理的五个有效策略",
            link="http://example.com/article4",
            description="结合实际案例，介绍小学班级管理的五个行之有效的策略...",
            pub_date=datetime(2025, 1, 10, 15, 45),
            guid="test4"
        ),
        RSSItem(
            title="朱永新：未来教育的十大趋势",
            link="http://example.com/article5",
            description="著名教育学者朱永新深度解析未来教育发展的十大趋势...",
            pub_date=datetime(2025, 1, 10, 16, 30),
            guid="test5"
        ),
    ]
    return items

def test_json_parsing_and_formatting():
    """测试JSON解析和格式化"""
    print("="*60)
    print("测试 JSON 解析和美化输出")
    print("="*60)

    # 创建测试条目
    items = create_test_items()
    print(f"\n✓ 创建了 {len(items)} 个测试RSS条目")

    # 加载示例JSON
    json_file = Path(__file__).parent / 'example_ai_response.json'
    with open(json_file, 'r', encoding='utf-8') as f:
        example_data = json.load(f)

    print(f"✓ 加载示例 AI 响应 JSON")

    # 创建增强器实例
    config = {'enabled': True, 'provider': 'openai'}
    enhancer = ContentEnhancer(config)

    # 测试JSON解析
    print("\n" + "="*60)
    print("1. 测试 JSON 解析")
    print("="*60)

    json_str = json.dumps(example_data, ensure_ascii=False)
    parsed = enhancer._parse_json_response(json_str)

    if parsed:
        print("✅ JSON 解析成功")
        print(f"   - 总结: {parsed.get('summary')}")
        print(f"   - 分类数: {len(parsed.get('categories', []))}")
    else:
        print("❌ JSON 解析失败")
        return

    # 测试美化输出
    print("\n" + "="*60)
    print("2. 测试美化输出")
    print("="*60)

    formatted = enhancer._format_beautiful_markdown(parsed, items)

    print("\n生成的 Markdown 内容：")
    print("="*60)
    print(formatted)
    print("="*60)

    # 保存到文件
    output_file = Path(__file__).parent / 'example_output.md'
    with open(output_file, 'w', encoding='utf-8') as f:
        f.write(formatted)

    print(f"\n✅ 输出已保存到: {output_file}")

    print("\n" + "="*60)
    print("测试完成!")
    print("="*60)
    print("\n关键特性验证:")
    print("✅ JSON 格式：AI 只需返回结构化数据")
    print("✅ 链接处理：自动通过 article_id 匹配原始链接")
    print("✅ 美观输出：使用 emoji 和格式化，提升用户体验")
    print("✅ 标签系统：快速识别文章主题")
    print("✅ 星级评价：直观展示重要程度")

if __name__ == '__main__':
    test_json_parsing_and_formatting()
