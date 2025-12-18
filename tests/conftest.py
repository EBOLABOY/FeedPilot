import pytest
import sys
from pathlib import Path

# Add src to path
sys.path.insert(0, str(Path(__file__).parent.parent / 'src'))

@pytest.fixture
def mock_rss_item():
    from src.models.rss_item import RSSItem
    from datetime import datetime
    
    item = RSSItem(
        title="测试文章",
        link="http://example.com/1",
        description="测试描述",
        pub_date=datetime.now(),
        guid="1"
    )
    return item
