import importlib

_strategy_module = importlib.import_module("freqtrade.strategy")
IStrategy = getattr(_strategy_module, "IStrategy")


class BraleSharedStrategy(IStrategy):
    minimal_roi = {"0": 10}
    # Hard safety net: close a trade once loss reaches 30%.
    # Reload or restart FreqTrade after changing this value.
    stoploss = -0.30
    timeframe = "5m"
    startup_candle_count = 50

    use_custom_stoploss = False
    use_custom_exit = False

    plot_config = {}

    def populate_indicators(self, dataframe, metadata):
        return dataframe

    def populate_entry_trend(self, dataframe, metadata):
        dataframe["enter_long"] = 0
        dataframe["enter_short"] = 0
        return dataframe

    def populate_exit_trend(self, dataframe, metadata):
        dataframe["exit_long"] = 0
        dataframe["exit_short"] = 0
        return dataframe
