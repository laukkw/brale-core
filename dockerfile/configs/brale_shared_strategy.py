"""BraleSharedStrategy - freqtrade 被动执行占位策略。

该策略完全关闭本地自动入场/止盈/止损逻辑，只保留必需的
populate_* 函数以满足 freqtrade 要求。所有实际的交易信号、
止盈止损与风控都由 Brale 进程通过 force-enter / force-exit 等
API 指令驱动，因此这里不需要读取共享数据库或执行额外的
线程任务。
"""

from freqtrade.strategy import IStrategy


class BraleSharedStrategy(IStrategy):
    minimal_roi = {"0": 10}
    stoploss = -0.99
    timeframe = "5m"
    startup_candle_count = 50

    use_custom_stoploss = False
    use_custom_exit = False

    plot_config = {}

    def populate_indicators(self, dataframe, metadata):
        # Brale 在容器外部负责一切决策，因此这里只是占位。
        return dataframe

    def populate_entry_trend(self, dataframe, metadata):
        dataframe["enter_long"] = 0
        dataframe["enter_short"] = 0
        return dataframe

    def populate_exit_trend(self, dataframe, metadata):
        dataframe["exit_long"] = 0
        dataframe["exit_short"] = 0
        return dataframe
