import pytest
from unittest.mock import MagicMock
from src.ai.content_enhancer import ContentEnhancer

def test_format_beautiful_markdown_digest_mode(mock_rss_item):
    """测试每日简报模式的 Markdown 生成"""
    config = {'enabled': True}
    enhancer = ContentEnhancer(config)
    
    # Mock data with summary_section
    ai_data = {
        "summary_section": {
            "title": "今日教育洞察",
            "insight": "这是深度洞察。",
            "trends": ["趋势1", "趋势2"]
        },
        "categories": [
            {
                "name": "必读",
                "level": 5,
                "articles": [
                    {"article_id": 1, "reason": "推荐理由", "tags": ["Tag1"]}
                ]
            }
        ]
    }
    
    items = [mock_rss_item]
    
    markdown = enhancer._format_beautiful_markdown(ai_data, items)
    
    assert "# 📚 育见-日报" in markdown
    assert "## 🧐 今日教育洞察" in markdown
    assert "这是深度洞察。" in markdown
    assert "**📉 关键趋势：**" in markdown
    assert "- 趋势1" in markdown
    assert "[测试文章]" in markdown

def test_format_beautiful_markdown_legacy_mode(mock_rss_item):
    """测试兼容旧模式"""
    config = {'enabled': True}
    enhancer = ContentEnhancer(config)
    
    ai_data = {
        "summary": "旧版总结",
        "categories": []
    }
    
    items = [mock_rss_item]
    markdown = enhancer._format_beautiful_markdown(ai_data, items)
    
    assert "**旧版总结**" in markdown
