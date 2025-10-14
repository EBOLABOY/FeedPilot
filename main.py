#!/usr/bin/env python3
"""
RSS推送服务主程序
定期抓取RSS源并推送到PushPlus群组
"""

import sys
import time
import schedule
from datetime import datetime
from pathlib import Path
from typing import List

# 添加src目录到路径
sys.path.insert(0, str(Path(__file__).parent / 'src'))

from src.config.loader import get_config
from src.utils.logger import configure_from_dict, get_logger
from src.rss.fetcher import RSSFetcher
from src.rss.parser import RSSParser
from src.pushers.pushplus import PushPlusPusher
from src.db.storage import PushStorage
from src.ai.content_enhancer import ContentEnhancer


class RSSPushService:
    """RSS推送服务主类"""

    def __init__(self, config_file: str = "config/app.yaml"):
        # 加载配置
        self.config = get_config(config_file)

        # 初始化日志
        log_config = self.config.get_logging_config()
        configure_from_dict(log_config)
        self.logger = get_logger(__name__)

        # 验证配置
        if not self.config.validate():
            raise ValueError("配置验证失败,请检查配置文件")

        # 初始化组件
        self.rss_config = self.config.get_rss_config()
        self.push_config = self.config.get_push_config()
        self.db_config = self.config.get_database_config()

        # RSS抓取器和解析器
        self.fetcher = RSSFetcher()
        self.parser = RSSParser(timezone_offset=8)

        # 数据库存储
        self.storage = PushStorage(self.db_config.get('path', 'data/pushed_items.db'))

        # 内容增强器
        enhancer_config = self.config.get('content_enhancer', {})
        self.content_enhancer = ContentEnhancer(enhancer_config)
        if enhancer_config.get('enabled', False):
            self.logger.info(f"内容增强器已启用: {self.content_enhancer}")

        # 推送器
        self.pushers = self._init_pushers()

        self.logger.info("RSS推送服务初始化完成")

    def _init_pushers(self) -> dict:
        """初始化推送器"""
        pushers = {}
        enabled_pushers = self.config.get_enabled_pushers()

        for pusher_name in enabled_pushers:
            try:
                if pusher_name == 'pushplus':
                    pushplus_config = self.config.get_pushplus_config()
                    pusher = PushPlusPusher(pushplus_config)
                    if pusher.initialize():
                        pushers[pusher_name] = pusher
                        self.logger.info(f"推送器 {pusher_name} 初始化成功")
                    else:
                        self.logger.error(f"推送器 {pusher_name} 初始化失败")
                else:
                    self.logger.warning(f"未知的推送器类型: {pusher_name}")

            except Exception as e:
                self.logger.error(f"初始化推送器 {pusher_name} 时发生异常: {e}")

        if not pushers:
            raise RuntimeError("没有可用的推送器")

        return pushers

    def _is_in_time_window(self) -> bool:
        """检查当前时间是否在推送时间窗口内"""
        if not self.config.is_time_window_enabled():
            return True

        time_window = self.config.get_time_window()
        now = datetime.now().time()

        start_time = datetime.strptime(time_window['start'], '%H:%M').time()
        end_time = datetime.strptime(time_window['end'], '%H:%M').time()

        if start_time <= end_time:
            return start_time <= now <= end_time
        else:
            # 跨午夜的时间窗口
            return now >= start_time or now <= end_time

    def fetch_and_push(self):
        """抓取RSS并推送(单次执行)"""
        try:
            self.logger.info("="*60)
            self.logger.info(f"开始执行RSS抓取和推送任务 - {datetime.now()}")

            # 检查时间窗口
            if not self._is_in_time_window():
                self.logger.info("当前不在推送时间窗口内,跳过本次推送")
                return

            # 1. 抓取RSS
            rss_url = self.rss_config.get('url')
            self.logger.info(f"正在抓取RSS源: {rss_url}")

            feed = self.fetcher.fetch_parsed(rss_url)
            if not feed or not feed.entries:
                self.logger.warning("未能获取RSS内容或RSS源为空")
                return

            # 2. 解析并转换为RSSItem
            from src.models.rss_item import RSSItem
            items = []
            for entry in feed.entries:
                try:
                    item = RSSItem.from_feedparser_entry(entry)
                    items.append(item)
                except Exception as e:
                    self.logger.error(f"解析RSS条目失败: {e}")

            self.logger.info(f"成功解析 {len(items)} 个RSS条目")

            # 3. 处理条目(去重、排序)
            # 不再按日期过滤,只要是RSS源中的内容都处理
            self.logger.info("开始处理RSS条目(去重和排序)")

            # 去重
            items = self.parser.deduplicate_items(items)

            # 按发布时间排序(最新的在前)
            items = self.parser.sort_by_publish_time(items, reverse=True)

            if not items:
                self.logger.info("没有有效的RSS条目")
                return

            # 4. 过滤未推送的条目(核心去重逻辑)
            unpushed_items = self.storage.filter_unpushed_items(items)

            if not unpushed_items:
                self.logger.info("所有内容均已推送,无需重复推送")
                return

            self.logger.info(f"发现 {len(unpushed_items)} 个未推送条目")

            # 5. 内容增强（对所有未推送条目进行AI分析）
            enhanced_content = None
            if self.content_enhancer.enabled:
                self.logger.info(f"开始对全部 {len(unpushed_items)} 条内容进行AI增强分析...")
                enhanced_content = self.content_enhancer.enhance_content(unpushed_items)

                if enhanced_content:
                    self.logger.info("AI内容增强完成")
                else:
                    self.logger.warning("AI内容增强失败，将推送原始内容")

            # 6. 分批推送（每次推送max_items条）
            pushplus_config = self.config.get_pushplus_config()
            max_items = pushplus_config.get('message_template', {}).get('max_items', 20)

            # 如果启用了AI增强且分析成功，推送AI增强内容（一次性推送所有）
            if enhanced_content:
                self.logger.info("推送AI增强后的内容...")
                for pusher_name, pusher in self.pushers.items():
                    try:
                        result = self._push_enhanced_content(pusher, enhanced_content, unpushed_items)

                        if result['success']:
                            self.logger.info(f"推送成功 - {pusher_name}: {result['message']}")
                            # 标记所有条目为已推送
                            self.storage.mark_items_as_pushed(
                                unpushed_items,
                                pusher_name=pusher_name,
                                success=True
                            )
                        else:
                            self.logger.error(f"推送失败 - {pusher_name}: {result['message']}")
                    except Exception as e:
                        self.logger.error(f"推送器 {pusher_name} 执行时发生异常: {e}")
            else:
                # 未启用AI或AI失败，分批推送原始内容
                total_items = len(unpushed_items)
                batch_count = (total_items + max_items - 1) // max_items  # 向上取整

                self.logger.info(f"将分 {batch_count} 批推送,每批最多 {max_items} 条")

                for batch_idx in range(batch_count):
                    start_idx = batch_idx * max_items
                    end_idx = min(start_idx + max_items, total_items)
                    items_batch = unpushed_items[start_idx:end_idx]

                    self.logger.info(f"正在推送第 {batch_idx + 1}/{batch_count} 批 ({len(items_batch)} 条)...")

                    for pusher_name, pusher in self.pushers.items():
                        try:
                            result = pusher.push_items(items_batch)

                            if result['success']:
                                self.logger.info(f"第 {batch_idx + 1} 批推送成功 - {pusher_name}")
                                self.storage.mark_items_as_pushed(
                                    items_batch,
                                    pusher_name=pusher_name,
                                    success=True
                                )
                            else:
                                self.logger.error(f"第 {batch_idx + 1} 批推送失败 - {pusher_name}: {result['message']}")
                        except Exception as e:
                            self.logger.error(f"推送器 {pusher_name} 执行第 {batch_idx + 1} 批时发生异常: {e}")

                    # 批次之间稍作延迟,避免频繁请求
                    if batch_idx < batch_count - 1:
                        self.logger.info("等待2秒后推送下一批...")
                        time.sleep(2)

            # 6. 显示统计信息
            stats = self.storage.get_statistics()
            self.logger.info(f"推送统计 - 总计: {stats.get('total_count', 0)}, "
                           f"今日: {stats.get('today_count', 0)}, "
                           f"本周: {stats.get('week_count', 0)}")

            self.logger.info("RSS抓取和推送任务完成")
            self.logger.info("="*60)

        except Exception as e:
            self.logger.error(f"执行RSS抓取和推送任务时发生异常: {e}", exc_info=True)

    def start_scheduler(self):
        """启动定时调度器"""
        scheduler_config = self.config.get_scheduler_config()

        if not scheduler_config.get('enabled', True):
            self.logger.warning("调度器未启用,仅执行一次后退出")
            self.fetch_and_push()
            return

        # 获取调度类型
        schedule_type = scheduler_config.get('schedule_type', 'interval')

        if schedule_type == 'daily':
            # 每天定时执行,支持多个时间点
            # 优先检查daily_times(多时间点),其次daily_time(单时间点)
            daily_times = scheduler_config.get('daily_times')
            if daily_times and isinstance(daily_times, list):
                # 多个推送时间点
                self.logger.info(f"启动定时调度器,每天在 {', '.join(daily_times)} 执行")

                for time_point in daily_times:
                    schedule.every().day.at(time_point).do(self.fetch_and_push)
                    self.logger.info(f"已设置定时任务: 每天 {time_point}")

                self.logger.info(f"共设置 {len(daily_times)} 个每日推送时间点")
            else:
                # 单个推送时间点(向后兼容)
                daily_time = scheduler_config.get('daily_time', '07:30')
                self.logger.info(f"启动定时调度器,每天 {daily_time} 执行一次")
                schedule.every().day.at(daily_time).do(self.fetch_and_push)
                self.logger.info(f"下次执行时间: {daily_time}")

            self.logger.info("等待定时任务触发...")

        else:
            # 按间隔执行(原有逻辑)
            fetch_interval = self.rss_config.get('fetch_interval', 5)
            self.logger.info(f"启动定时调度器,每 {fetch_interval} 分钟执行一次")

            # 立即执行一次
            self.fetch_and_push()

            # 设置定时任务
            schedule.every(fetch_interval).minutes.do(self.fetch_and_push)

        # 运行调度器
        try:
            while True:
                schedule.run_pending()
                time.sleep(1)
        except KeyboardInterrupt:
            self.logger.info("收到退出信号,正在关闭服务...")
            self.cleanup()

    def test_connection(self):
        """测试推送器连接"""
        self.logger.info("开始测试推送器连接...")

        for pusher_name, pusher in self.pushers.items():
            result = pusher.test_connection()
            if result['success']:
                self.logger.info(f"✓ {pusher_name}: {result['message']}")
            else:
                self.logger.error(f"✗ {pusher_name}: {result['message']}")

    def show_statistics(self):
        """显示推送统计信息"""
        stats = self.storage.get_statistics()

        print("\n" + "="*60)
        print("RSS推送服务统计信息")
        print("="*60)
        print(f"总推送次数: {stats.get('total_count', 0)}")
        print(f"成功推送: {stats.get('success_count', 0)}")
        print(f"失败推送: {stats.get('failed_count', 0)}")
        print(f"今日推送: {stats.get('today_count', 0)}")
        print(f"本周推送: {stats.get('week_count', 0)}")
        print(f"最后推送时间: {stats.get('last_pushed', '无')}")
        print("="*60 + "\n")

    def _push_enhanced_content(self, pusher, enhanced_content: str, items: List) -> dict:
        """
        推送增强后的内容
        :param pusher: 推送器实例
        :param enhanced_content: 增强后的Markdown内容
        :param items: 原始RSS条目（用于记录）
        :return: 推送结果
        """
        try:
            # 使用PushPlus的自定义消息推送
            if hasattr(pusher, 'push_custom_message'):
                return pusher.push_custom_message(
                    title="📚 深圳教师社招主观热点素材",
                    content=enhanced_content,
                    template="markdown"  # 使用markdown格式
                )
            else:
                # 如果推送器不支持自定义消息，降级为普通推送
                self.logger.warning(f"推送器不支持自定义消息，使用标准格式")
                return pusher.push_items(items)

        except Exception as e:
            self.logger.error(f"推送增强内容失败: {e}")
            return {
                'success': False,
                'message': f"推送失败: {str(e)}"
            }

    def cleanup(self):
        """清理资源"""
        self.logger.info("正在清理资源...")
        self.fetcher.close()
        self.storage.close()
        self.logger.info("资源清理完成")


def main():
    """主函数"""
    import argparse

    parser = argparse.ArgumentParser(description='RSS推送服务')
    parser.add_argument(
        '--config',
        default='config/app.yaml',
        help='配置文件路径 (默认: config/app.yaml)'
    )
    parser.add_argument(
        '--once',
        action='store_true',
        help='仅执行一次,不启动定时调度'
    )
    parser.add_argument(
        '--test',
        action='store_true',
        help='测试推送器连接'
    )
    parser.add_argument(
        '--stats',
        action='store_true',
        help='显示推送统计信息'
    )
    parser.add_argument(
        '--cleanup',
        type=int,
        metavar='DAYS',
        help='清理N天前的旧推送记录'
    )

    args = parser.parse_args()

    try:
        # 创建服务实例
        service = RSSPushService(config_file=args.config)

        # 根据参数执行不同操作
        if args.test:
            service.test_connection()
        elif args.stats:
            service.show_statistics()
        elif args.cleanup:
            deleted = service.storage.cleanup_old_records(days=args.cleanup)
            print(f"已清理 {deleted} 条超过 {args.cleanup} 天的旧记录")
        elif args.once:
            service.fetch_and_push()
            service.cleanup()
        else:
            # 启动定时调度器
            service.start_scheduler()

    except KeyboardInterrupt:
        print("\n程序被用户中断")
        sys.exit(0)
    except Exception as e:
        print(f"程序运行失败: {e}")
        sys.exit(1)


if __name__ == '__main__':
    main()
