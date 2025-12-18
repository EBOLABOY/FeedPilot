import sys
import time
import schedule
from datetime import datetime
from pathlib import Path
from typing import List
from zoneinfo import ZoneInfo

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

    def _build_daily_schedule_times(self, scheduler_config) -> list[tuple[str, str]]:
        """
        根据配置的调度时间与时区,构建 (配置时间, 容器本地执行时间) 列表.
        """
        # 1. 先整理出“配置时间列表”(配置时区视角)
        daily_times = scheduler_config.get('daily_times')
        if daily_times and isinstance(daily_times, list):
            config_times = [t.strip() for t in daily_times if t and t.strip()]
        else:
            # 单个时间点(向后兼容)
            daily_time = scheduler_config.get('daily_time', '07:30')
            config_times = [daily_time]

        timezone_name = scheduler_config.get('timezone')
        if not timezone_name:
            # 未配置时区,直接按容器本地时间执行
            return [(t, t) for t in config_times]

        # 2. 尝试按配置时区转换到容器本地时间
        schedule_pairs: list[tuple[str, str]] = []
        try:
            config_tz = ZoneInfo(timezone_name)
        except Exception as e:  # pragma: no cover
            self.logger.error(f"无法识别调度时区配置 scheduler.timezone='{timezone_name}', 将按容器本地时间执行: {e}")
            return [(t, t) for t in config_times]

        # 容器本地时区
        local_tz = datetime.now().astimezone().tzinfo

        for t in config_times:
            try:
                hour, minute = map(int, t.split(':'))
            except ValueError:
                self.logger.error(f"无效的调度时间格式: '{t}', 应为 HH:MM")
                continue

            today_in_config_tz = datetime.now(config_tz).date()
            config_dt = datetime(
                year=today_in_config_tz.year,
                month=today_in_config_tz.month,
                day=today_in_config_tz.day,
                hour=hour,
                minute=minute,
                tzinfo=config_tz,
            )

            local_dt = config_dt.astimezone(local_tz)
            local_time_str = local_dt.strftime('%H:%M')

            schedule_pairs.append((t, local_time_str))
            if t == local_time_str:
                self.logger.info(
                    f"调度时间: 每天 {t} (配置时区={timezone_name}, 与容器本地时间一致)"
                )
            else:
                self.logger.info(
                    f"调度时间转换: 配置时区 {timezone_name} 的 {t} "
                    f"对应容器本地时间 {local_time_str}"
                )

        if not schedule_pairs:
            return [(t, t) for t in config_times]

        return schedule_pairs

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
            self.logger.info("开始处理RSS条目(去重和排序)")
            items = self.parser.deduplicate_items(items)
            items = self.parser.sort_by_publish_time(items, reverse=True)

            if not items:
                self.logger.info("没有有效的RSS条目")
                return

            # 4. 过滤未推送的条目
            unpushed_items = self.storage.filter_unpushed_items(items)

            if not unpushed_items:
                self.logger.info("所有内容均已推送,无需重复推送")
                return

            self.logger.info(f"发现 {len(unpushed_items)} 个未推送条目")

            # 5. 内容增强（AI分析）
            enhanced_content = None
            if self.content_enhancer.enabled:
                self.logger.info(f"开始对全部 {len(unpushed_items)} 条内容进行AI增强分析...")
                enhanced_content = self.content_enhancer.enhance_content(unpushed_items)

                if enhanced_content:
                    self.logger.info("AI内容增强完成")
                else:
                    self.logger.warning("AI内容增强失败，将推送原始内容")

            # 6. 推送
            self.logger.info("开始推送流程...")
            for pusher_name, pusher in self.pushers.items():
                try:
                    # 如果有AI增强内容，优先推送增强后的内容
                    if enhanced_content:
                        result = self._push_enhanced_content(pusher, enhanced_content, unpushed_items)
                    else:
                        # 降级为普通分批推送
                        result = self._push_normal_items(pusher, unpushed_items)

                    if result['success']:
                         # 统一标记成功 (如果是分批推送, 已经在 _push_normal_items 里标记了, 但这里再次全量标记也无害; 
                         # 实际上 _push_enhanced_content 成功意味着所有 items 都被 summarize 了)
                         if enhanced_content:
                            self.storage.mark_items_as_pushed(
                                unpushed_items,
                                pusher_name=pusher_name,
                                success=True
                            )
                    else:
                        self.logger.error(f"推送失败 - {pusher_name}: {result['message']}")

                except Exception as e:
                     self.logger.error(f"推送器 {pusher_name} 执行异常: {e}")

            # 6. 显示统计信息
            stats = self.storage.get_statistics()
            self.logger.info(f"推送统计 - 总计: {stats.get('total_count', 0)}, "
                           f"今日: {stats.get('today_count', 0)}, "
                           f"本周: {stats.get('week_count', 0)}")

            self.logger.info("RSS抓取和推送任务完成")
            self.logger.info("="*60)

        except Exception as e:
            self.logger.error(f"执行RSS抓取和推送任务时发生异常: {e}", exc_info=True)

    def _push_normal_items(self, pusher, items) -> dict:
        """普通分批推送 helper"""
        pushplus_config = self.config.get_pushplus_config()
        max_items = pushplus_config.get('message_template', {}).get('max_items', 20)
        total_items = len(items)
        batch_count = (total_items + max_items - 1) // max_items

        self.logger.info(f"将分 {batch_count} 批推送普通内容,每批最多 {max_items} 条")
        
        last_result = {'success': True, 'message': 'All batches processed'}

        for batch_idx in range(batch_count):
            start_idx = batch_idx * max_items
            end_idx = min(start_idx + max_items, total_items)
            items_batch = items[start_idx:end_idx]

            result = pusher.push_items(items_batch)
            last_result = result 

            if result['success']:
                self.storage.mark_items_as_pushed(
                    items_batch,
                    pusher_name=type(pusher).__name__, # 简化
                    success=True
                )
            
            if batch_idx < batch_count - 1:
                time.sleep(2)
        
        return last_result

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
            timezone_name = scheduler_config.get('timezone', '本地时间')

            # 计算(配置时区时间 -> 容器本地执行时间)映射
            schedule_pairs = self._build_daily_schedule_times(scheduler_config)
            config_times_display = [cfg for cfg, _ in schedule_pairs]

            self.logger.info(
                f"启动定时调度器,每天在 {', '.join(config_times_display)} 执行 "
                f"(配置时区: {timezone_name})"
            )

            for cfg_time, local_time in schedule_pairs:
                schedule.every().day.at(local_time).do(self.fetch_and_push)
                if cfg_time == local_time or scheduler_config.get('timezone') is None:
                    # 未配置时区,或配置时区与容器本地时区一致
                    self.logger.info(f"已设置定时任务: 每天 {local_time}")
                else:
                    self.logger.info(
                        f"已设置定时任务: 每天 {cfg_time} (配置时区) / "
                        f"容器本地执行时间 {local_time}"
                    )

            self.logger.info(f"共设置 {len(schedule_pairs)} 个每日推送时间点")

            # 启动时立即执行一次,与按间隔调度行为保持一致
            self.logger.info("服务启动后立即执行一次RSS抓取和推送任务")
            self.fetch_and_push()

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
        """
        try:
            # 使用PushPlus的自定义消息推送
            if hasattr(pusher, 'push_custom_message'):
                return pusher.push_custom_message(
                    title="📚 深圳教师社招每日简报",  # 更新标题
                    content=enhanced_content,
                    template="markdown"
                )
            else:
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
