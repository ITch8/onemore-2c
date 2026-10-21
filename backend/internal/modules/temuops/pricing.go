package temuops

// PriceQuote is a deterministic follow-price result in minor currency units.
type PriceQuote struct {
	SuggestedPriceMinor int64
	FloorMinor          int64
	Reason              string
}

// QuoteFollowPrice keeps a competitor follow-price inside the cost margin and the min/max bounds.
// Minor units are 1/100 of the currency. A zero competitor price skips follow-down.
func QuoteFollowPrice(costMinor int64, marginBps int, minMinor, maxMinor, competitorMinor, deltaMinor int64) (PriceQuote, error) {
	if costMinor < 0 || minMinor < 0 || maxMinor < 0 || competitorMinor < 0 || deltaMinor < 0 {
		return PriceQuote{}, invalid("价格不能为负数")
	}
	if marginBps < 0 || marginBps > 100000 {
		return PriceQuote{}, invalid("毛利率超出允许范围")
	}
	if maxMinor > 0 && minMinor > maxMinor {
		return PriceQuote{}, invalid("最低价不能高于最高价")
	}
	floor := costMinor * int64(10000+marginBps) / 10000
	if minMinor > floor {
		floor = minMinor
	}
	suggested := floor
	reason := "按成本毛利和最低价计算"
	if competitorMinor > 0 {
		target := competitorMinor - deltaMinor
		if target < floor {
			target = floor
			reason = "跟价后低于底价，已抬回到底价"
		} else {
			reason = "按竞品价减去跟价差额，且不低于底价"
		}
		suggested = target
	}
	if maxMinor > 0 && suggested > maxMinor {
		if floor > maxMinor {
			return PriceQuote{}, invalid("底价高于最高价，无法给出可执行售价")
		}
		suggested = maxMinor
		reason = "建议价高于最高价，已压到最高价"
	}
	return PriceQuote{SuggestedPriceMinor: suggested, FloorMinor: floor, Reason: reason}, nil
}

// QuoteAdBudget suggests a budget change from spend and revenue. It never applies the change.
func QuoteAdBudget(current, spend, revenue int64, targetRoasBps int) (suggested int64, reason string, err error) {
	if current < 0 || spend < 0 || revenue < 0 || targetRoasBps < 0 {
		return 0, "", invalid("广告金额不能为负数")
	}
	if current == 0 {
		return 0, "当前预算为 0，保持不变", nil
	}
	if spend == 0 {
		return current, "尚无花费，保持当前预算", nil
	}
	roas := revenue * 10000 / spend
	if roas < int64(targetRoasBps) {
		next := current * 80 / 100
		if next < 1 {
			next = 1
		}
		return next, "回报低于目标，建议将预算下调两成，需人工确认后才记录执行", nil
	}
	next := current * 110 / 100
	cap := current * 2
	if next > cap {
		next = cap
	}
	return next, "回报达到目标，建议将预算上调一成，需人工确认后才记录执行", nil
}
