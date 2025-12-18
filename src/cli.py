import sys
import argparse
from .service import RSSPushService


def main():
    """主函数"""
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
