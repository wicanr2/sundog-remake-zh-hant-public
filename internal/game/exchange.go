package game

import (
	"strconv"

	"github.com/wicanr2/sundog-remake/internal/world"
)

// The exchange the way XDOTRADI runs it.
//
// A lot (a pod slot, a warehouse slot, a quote) is two words: the goods in
// five bits, the quality in three above them, and the price paid (or asked)
// in the second word. The pod holds two lots (MAINLIB_Gec), each city's
// warehouse four (XDOTRADI_01_L39, kept in the trade database by city).
//
// get_data reads the city's demand table (L1b) and supply table (L22): one
// byte per goods, each seeded on the first visit by init_sd -- a roll up to
// the class table's figure, sometimes a little over -- and aged by adjust
// every seven days toward the class figure. add_quote fills the ring of
// eight quotes: goods drawn in a random order, skipping those with no
// supply and those already quoted, kept when a roll of seven comes in
// under the supply; quality 1 + two rolls of four; price from
// current_price. update_market moves a table entry by a roll of three when
// a lot changes hands.
//
// current_price (0x12): start from the goods' base value (item record byte
// 15), then factor(demand, supply), factor(priceLevel/2, 4),
// factor(quality, 6), for goods 16–24 factor(9, streetDealRating), then the
// side of the trade, and a seller gives up a twentieth. factor(p, a, b) is
// p × (8 + |a−b|) / 8 when a ≥ b and p × 8 / (8 + |a−b|) otherwise.
//
// The class table init_sd and adjust work from is **logical block 7**, and
// since 2026-09-06 the remake reads it: eighteen records, one a planet, each
// carrying a three-bit supply and demand class for goods 1–30 plus the
// planet's own produce (docs/atari-st/trade-classes.json). get_data picks a
// city's record as ((planet - 1 + saved global shift) mod 18), then applies
// the planet's word6 supply and word13 demand overrides. So a city's market starts where the
// disk says and climbs back to it, instead of starting from a roll of eight
// and drifting to the middle.
//
// The side factor is 8/7 for sales outside the planet's first source city,
// and 7/8 otherwise. See docs/atari-st/banville-restoration-spec.md for
// the scoped price evidence. Origin-distance adjustment (find_location) is
// supported for held lots; the complete Banville trade/save chain remains pending.

// Lot is one lot of goods: a pod slot, a warehouse slot or a quote.
type Lot struct {
	SourceCode uint8 `json:"sourceCode"`
	Goods      int   `json:"goods"`
	Quality    int   `json:"quality"`
	// Price is what was paid for a held lot, or what is asked for a quote.
	Price int `json:"price"`
}

// CityMarket is a city's supply and demand tables and the day they were
// last aged.
type CityMarket struct {
	Supply   []int `json:"supply"`
	Demand   []int `json:"demand"`
	LastSeen int   `json:"lastSeen"`
}

const (
	QuoteRing       = 8
	WarehouseLots   = 4
	goodsCodes      = 32
	marketSeedSpan  = 8
	marketAgeDays   = 7
	quoteAcceptSpan = 7
	qualityRollSpan = 4
	marketMoveSpan  = 3
	// CreditsPerPriceUnit is the original's price scale. A quote's price word is
	// in hundreds: the exchange prints it as-is on the marquee ("gold-120") but
	// multiplies by 100 for the Asking: line ("4,300" for a price word of 43),
	// and cash is in plain credits — On hand: 202 at a new game
	// (docs/atari-st/ux-scene-ledger.md, eighth round).
	CreditsPerPriceUnit = 100
	priceFactorBase     = 8
	priceLevelDivide    = 2
	priceLevelPivot     = 4
	qualityPivot        = 6
	unsafeGoodsLow      = 16
	unsafeGoodsHigh     = 24
	unsafePivot         = 9
	sellerCut           = 20
	marqueeCell         = 21
)

// tradeRoll is the exchange's die, its own counter so the fight's rolls
// do not move the market.
func tradeRoll(state *State, span int) int {
	if span <= 0 {
		return 0
	}
	state.TradeRolls++
	mixed := (uint32(state.TradeRolls) + citySalt(state.CityID)) * 22695477
	mixed ^= mixed >> 15
	mixed *= 2654435761
	mixed ^= mixed >> 13
	return int(mixed % uint32(span))
}

// citySalt keeps two cities' first visits from rolling the same tables: the
// original seeds from its clock, the remake from the city's name.
func citySalt(cityID string) uint32 {
	hash := uint32(2166136261)
	for i := 0; i < len(cityID); i++ {
		hash ^= uint32(cityID[i])
		hash *= 16777619
	}
	return hash
}

// cityMarket is the city's tables, seeded on the first visit (init_sd) and
// aged since the last (adjust).
func (r Reducer) cityMarket(state *State, cityID string) CityMarket {
	if state.Markets == nil {
		state.Markets = map[string]CityMarket{}
	}
	market, ok := state.Markets[cityID]
	day := state.TimeMinutes / (24 * 60)
	classes, hasClasses := world.TradeClassesFor(cityID, state.TradeClassShift)
	if cityID == world.MissionDestinationID {
		if !ok {
			market = CityMarket{Supply: make([]int, goodsCodes), Demand: make([]int, goodsCodes)}
			if hasClasses {
				copy(market.Supply[1:31], classes.Supply)
			}
		}
		// Banville 的需求由已保存的階段表主導，不用普通城市 RND 或週老化。
		market.Demand = append([]int(nil), state.Colony.Remaining[:]...)
		state.Markets[cityID] = market
		return market
	}
	if !ok {
		market = CityMarket{Supply: make([]int, goodsCodes), Demand: make([]int, goodsCodes), LastSeen: day}
		for goods := 1; goods <= 30; goods++ {
			if hasClasses {
				market.Supply[goods] = seedFromClass(state, classFor(classes.Supply, goods))
				market.Demand[goods] = seedFromClass(state, classFor(classes.Demand, goods))
				continue
			}
			market.Supply[goods] = tradeRoll(state, marketSeedSpan)
			market.Demand[goods] = tradeRoll(state, marketSeedSpan)
		}
	} else if weeks := (day - market.LastSeen) / marketAgeDays; weeks > 0 {
		// adjust: each entry climbs back toward its class, and stops there.
		for goods := 1; goods <= 30; goods++ {
			supplyTarget, demandTarget := marketSeedSpan/2, marketSeedSpan/2
			if hasClasses {
				supplyTarget = classFor(classes.Supply, goods)
				demandTarget = classFor(classes.Demand, goods)
			}
			market.Supply[goods] = ageUpTo(market.Supply[goods], supplyTarget, weeks)
			market.Demand[goods] = ageUpTo(market.Demand[goods], demandTarget, weeks)
		}
		market.LastSeen = day
	}
	state.Markets[cityID] = market
	return market
}

// classFor is one goods' class, or the middle of the range when the goods is
// outside the table's thirty.
func classFor(classes []int, goods int) int {
	if goods < 1 || goods > len(classes) {
		return marketSeedSpan / 2
	}
	return classes[goods-1]
}

// seedFromClass is init_sd: a roll up to the class, nudged up half the time
// when it came in under, then clamped back into [0, class].
func seedFromClass(state *State, class int) int {
	value := tradeRoll(state, class+1)
	if value < class && tradeRoll(state, 2) == 1 {
		value += tradeRoll(state, marketMoveSpan+1)
	}
	return clamp(value, 0, class)
}

// ageUpTo is adjust: one step a period upward, stopping at the class. The
// original only ever climbs — a market that has been traded down recovers,
// and one that is already at its class stays there.
func ageUpTo(value, target, steps int) int {
	value += steps
	if value > target {
		value = target
	}
	return value
}

func ageToward(value, target, steps int) int {
	for ; steps > 0 && value != target; steps-- {
		if value < target {
			value++
		} else {
			value--
		}
	}
	return value
}

// MaxFuel is a full tank. A new game starts empty (worklist D6), so the shells'
// diagnostic shortcuts need a way to fill it.
func MaxFuel() int { return maxFuel }

// Credits turns a quote's price word into the credits it actually costs.
func Credits(price int) int { return price * CreditsPerPriceUnit }

// FormatCredits writes an amount the way the original writes it: a comma every
// three digits. Both of the Uniteller capture's readouts follow it —
// "On hand: 202" has no comma because it is under a thousand, "In bank: 36,306"
// has one — and so does the exchange's "Asking: 4,300". The evidence is those
// captures, not a decoded routine; the original's price line reaches the same
// shape by a different road (it divides the price word by ten and prints the
// remainder times a hundred, which is the ×100 unit and the comma at once).
func FormatCredits(value int) string {
	digits := strconv.Itoa(value)
	sign := ""
	if digits[0] == '-' {
		sign, digits = "-", digits[1:]
	}
	out := make([]byte, 0, len(digits)+len(digits)/3)
	for i := 0; i < len(digits); i++ {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, digits[i])
	}
	return sign + string(out)
}

// priceFactor is current_price:factor.
func priceFactor(price, a, b int) int {
	diff := priceFactorBase + abs(a-b)
	if a >= b {
		return priceMultiplyDivide(price, diff, priceFactorBase)
	}
	return priceMultiplyDivide(price, priceFactorBase, diff)
}

// XDOTRADI:14，段內0x0850..0x0867；溢位邊界先除再乘。
func priceMultiplyDivide(price, multiplier, divisor int) int {
	if 32767/multiplier <= price {
		return multiplier * (price / divisor)
	}
	return price * multiplier / divisor
}

// CurrentPrice is current_price for goods of a quality at the city, asked of
// a buyer or offered to a seller.
func (r Reducer) CurrentPrice(state *State, goods, quality int, selling bool) (int, bool) {
	return r.CurrentLotPrice(state, Lot{Goods: goods, Quality: quality}, selling)
}

// CurrentLotPrice retains the original origin-city byte and purchase-price cap.
func (r Reducer) CurrentLotPrice(state *State, lot Lot, selling bool) (int, bool) {
	if _, ok := r.World.CargoKindByCode(lot.Goods); !ok {
		return 0, false
	}
	if state.CityID == world.MissionDestinationID {
		classes, ok := world.TradeClassesFor(state.CityID, state.TradeClassShift)
		if !ok {
			return 0, false
		}
		supply, demand := 0, 0
		if lot.Goods <= 30 {
			supply, demand = classes.Supply[lot.Goods-1], classes.Demand[lot.Goods-1]
		}
		return r.currentLotPriceForSupplyDemand(state, lot, selling, supply, demand)
	}
	market := r.cityMarket(state, state.CityID)
	return r.currentLotPriceForSupplyDemand(state, lot, selling, market.Supply[lot.Goods], market.Demand[lot.Goods])
}

// 共用估值算術；供需來源由正式CurrentLotPrice選擇，不接受玩家覆寫分類。
func (r Reducer) currentLotPriceForSupplyDemand(state *State, lot Lot, selling bool, supply, demand int) (int, bool) {
	goods, quality := lot.Goods, lot.Quality
	kind, ok := r.World.CargoKindByCode(goods)
	if !ok {
		return 0, false
	}
	price := kind.BaseValue
	price = priceFactor(price, demand, supply)
	system, _ := r.World.System(state.SystemID)
	price = priceFactor(price, system.PriceLevel/priceLevelDivide, priceLevelPivot)
	price = priceFactor(price, quality, qualityPivot)
	if goods >= unsafeGoodsLow && goods <= unsafeGoodsHigh {
		price = priceFactor(price, unsafePivot, system.StreetDealRating)
	}
	destination, found := r.World.Destination(state.CityID)
	cities := r.World.DestinationsOnPlanet(destination.PlanetID)
	if selling && found && len(cities) > 0 && cities[0].ID != state.CityID {
		price = priceFactor(price, priceFactorBase, priceFactorBase-1)
	} else {
		price = priceFactor(price, priceFactorBase-1, priceFactorBase)
	}
	limit := price
	if selling && lot.SourceCode != 0 {
		limit = lot.Price
		if !found || int(lot.SourceCode) != destination.SourceIndex {
			origin, known := r.World.CargoOrigin(lot.SourceCode)
			switch {
			case !known || origin.SystemID != state.SystemID:
				limit = price
			case origin.ID == destination.PlanetID:
				limit = priceFactor(limit, 8, 6)
			default:
				limit = priceFactor(limit, 8, 4)
			}
		}
	}
	price = priceCondition(price, limit)
	if selling {
		cut := price / sellerCut
		if cut == 0 {
			cut = 1
		}
		price -= cut
	}
	price = priceCondition(price, limit)
	return price, true
}

// MAINLIB:0a tests the upper bound first, including upper < lower.
func priceCondition(price, upper int) int {
	if price > upper {
		return upper
	}
	if price < 1 {
		return 1
	}
	return price
}

// openExchange is get_data and the first eight add_quote calls: the tables
// for the city and a fresh ring.
func (r Reducer) openExchange(state *State) {
	beginTradeSession(state)
	if state.CityID == world.MissionDestinationID {
		advanceColonyAtExchange(state)
		if state.MissionComplete {
			return
		}
	}
	r.cityMarket(state, state.CityID)
	state.Quotes = make([]Lot, QuoteRing)
	for i := 0; i < QuoteRing; i++ {
		r.addQuote(state, i)
	}
	state.QuoteIndex = 0
}

// addQuote fills one slot of the ring.
func (r Reducer) addQuote(state *State, slot int) {
	market := r.cityMarket(state, state.CityID)
	order := make([]int, 0, goodsCodes-1)
	for goods := 1; goods < goodsCodes; goods++ {
		if _, ok := r.World.CargoKindByCode(goods); ok {
			order = append(order, goods)
		}
	}
	// A random draw order, as add_quote's shuffle.
	for i := len(order) - 1; i > 0; i-- {
		j := tradeRoll(state, i+1)
		order[i], order[j] = order[j], order[i]
	}
	chosen := 0
	for _, goods := range order {
		if market.Supply[goods] <= 0 || r.inBids(*state, slot, goods) {
			continue
		}
		if market.Supply[goods] > tradeRoll(state, quoteAcceptSpan) {
			chosen = goods
			break
		}
		if chosen == 0 {
			chosen = goods
		}
	}
	if chosen == 0 {
		state.Quotes[slot] = Lot{}
		return
	}
	quality := 1 + tradeRoll(state, qualityRollSpan) + tradeRoll(state, qualityRollSpan)
	price, _ := r.CurrentPrice(state, chosen, quality, false)
	state.Quotes[slot] = Lot{Goods: chosen, Quality: quality, Price: price}
}

// inBids is add_quote:in_bids: the goods is already in another slot.
func (r Reducer) inBids(state State, slot, goods int) bool {
	for i, quote := range state.Quotes {
		if i != slot && quote.Goods == goods {
			return true
		}
	}
	return false
}

// updateMarket is update_market: a lot bought lowers the supply, a lot
// sold lowers the demand, each by a roll of three.
func (r Reducer) updateMarket(state *State, goods int, sold bool) {
	market := r.cityMarket(state, state.CityID)
	table := market.Supply
	if sold {
		table = market.Demand
	}
	// XDOTRADI:0x16 0x0a63..0x0a77：Banville 每筆固定扣 1，
	// 不消耗市場亂數；其他城市才減 random mod 3。
	decrease := 1
	if state.CityID != world.MissionDestinationID {
		decrease = tradeRoll(state, marketMoveSpan)
	}
	table[goods] -= decrease
	if table[goods] < 0 {
		table[goods] = 0
	}
	state.Markets[state.CityID] = market
}

// CurrentQuote is the quote the Buy page shows, if the ring has one.
func CurrentQuote(state State) (Lot, bool) {
	for i := 0; i < len(state.Quotes); i++ {
		quote := state.Quotes[(state.QuoteIndex+i)%len(state.Quotes)]
		if quote.Goods != 0 {
			return quote, true
		}
	}
	return Lot{}, false
}

// passQuote moves the ring on to the next quote (do_buying's Pass).
func passQuote(state *State) {
	if len(state.Quotes) == 0 {
		return
	}
	for i := 1; i <= len(state.Quotes); i++ {
		index := (state.QuoteIndex + i) % len(state.Quotes)
		if state.Quotes[index].Goods != 0 {
			state.QuoteIndex = index
			return
		}
	}
}

// buyQuote replaces the approved quote and pays before choosing its storage.
// Foot purchases go to the warehouse; pod purchases wait for a destination.
func (r Reducer) buyQuote(state *State) (string, bool) {
	if !TradePurchaseFits(*state) {
		return purchaseFullMessage(*state), false
	}
	quote, ok := CurrentQuote(*state)
	if !ok || quote.Price <= 0 {
		return "ui.message.no_lots", false
	}
	cost := Credits(quote.Price)
	if TradeFunds(*state) < cost {
		return "ui.message.insufficient_cash", false
	}
	_, ok = r.World.CargoKindByCode(quote.Goods)
	if !ok {
		return "ui.message.no_lots", false
	}
	destination, found := r.World.Destination(state.CityID)
	if !found {
		return "ui.message.no_lots", false
	}
	lot := Lot{Goods: quote.Goods, Quality: quote.Quality, Price: quote.Price, SourceCode: uint8(destination.SourceIndex)}
	// XDOTRADI 130e updates the selected quote before payment. The paid lot
	// remains separate from the replacement while pick_item chooses its slot.
	r.addQuote(state, state.QuoteIndex)
	spendTradeFunds(state, cost)
	state.TradePurchaseLot = &lot
	if state.GroundMode == GroundInPod {
		state.TradePage = TradePageBuyDestination
		return "ui.message.trade_purchase_destination", true
	}
	return r.finishTradePurchase(state, false)
}

func firstWarehouseFree(state State) int {
	for i, lot := range WarehouseLotsOf(state, state.CityID) {
		if lot.Goods == 0 {
			return i
		}
	}
	return -1
}

// TradePurchaseFits is the original 122c capacity gate, before quote/input.
func TradePurchaseFits(state State) bool {
	return firstWarehouseFree(state) >= 0 || state.GroundMode == GroundInPod && CargoLotsFree(state) > 0
}

func purchaseFullMessage(state State) string {
	if state.GroundMode == GroundInPod {
		return "ui.message.cargo_full"
	}
	return "ui.message.warehouse_full"
}

func (r Reducer) finishTradePurchase(state *State, pod bool) (string, bool) {
	if state.TradePurchaseLot == nil {
		return "ui.message.no_lots", false
	}
	lot := *state.TradePurchaseLot
	free := firstWarehouseFree(*state)
	if pod && CargoLotsFree(*state) <= 0 {
		pod = false
	} else if !pod && free < 0 && state.GroundMode == GroundInPod {
		pod = true
	}
	if pod {
		kind, ok := r.World.CargoKindByCode(lot.Goods)
		if !ok || !addPodLot(state, lot, kind.Key) {
			return purchaseFullMessage(*state), false
		}
	} else {
		if free < 0 {
			return purchaseFullMessage(*state), false
		}
		slots := warehouse(state, state.CityID)
		slots[free] = lot
		state.Warehouses[state.CityID] = slots
	}
	state.TradePurchaseLot = nil
	r.updateMarket(state, lot.Goods, false)
	passQuote(state)
	state.TradePage = TradePageMain
	return "ui.message.bought", true
}

// addPodLot puts a lot in the first free hold.
func addPodLot(state *State, lot Lot, key string) bool {
	if !loadHold(state, goodsHold(lot, key)) {
		return false
	}
	state.Gems = len(PodLots(*state))
	return true
}

// sellPodLot sells one of the pod's two holds at the seller's price. The
// index is a stable slot. The original pick_item also lists four warehouse
// slots, which this entry does not support. Ordinary income enters the transaction's available funds;
// the exchange settles bank and cash only when it returns to the ground.
func (r Reducer) sellPodLot(state *State, holdIndex int) (string, bool) {
	hold := HoldAt(*state, holdIndex)
	if hold.Kind != HoldGoods || hold.Goods == 0 {
		return "ui.message.no_cargo", false
	}
	lot := hold.Lot()
	price, ok := r.CurrentLotPrice(state, lot, true)
	if !ok {
		return "ui.message.no_cargo", false
	}
	if !r.settleApprovedLot(state, lot, price) {
		return "ui.message.trade_not_needed", false
	}
	if state.Screen == ScreenScore {
		return "ui.message.mission_complete", true
	}
	// XDOTRADI :0x1b 11fa..1225 clears all four bytes of the sold record.
	// Fuel consumption and jettison retain metadata through emptyHold.
	clearTradeSaleSlot(state, holdIndex+5)
	return "ui.message.sold", true
}

// sellWarehouseLot uses the same price and funds rules as the pod sale.
// XDOTRADI :1a 1135..1143 selects a warehouse record; :1b clears it afterward.
func (r Reducer) sellWarehouseLot(state *State, index int) (string, bool) {
	if index < 0 || index >= WarehouseLots {
		return "ui.message.no_cargo", false
	}
	lot := WarehouseLotsOf(*state, state.CityID)[index]
	if lot.Goods == 0 {
		return "ui.message.no_cargo", false
	}
	price, ok := r.CurrentLotPrice(state, lot, true)
	if !ok {
		return "ui.message.no_cargo", false
	}
	if !r.settleApprovedLot(state, lot, price) {
		return "ui.message.trade_not_needed", false
	}
	if state.Screen == ScreenScore {
		return "ui.message.mission_complete", true
	}
	clearTradeSaleSlot(state, index+1)
	return "ui.message.sold", true
}

// XDOTRADI:1a 的穩定 selector，1..4 倉庫，5..6 Pod。
func PendingTradeLot(s State) (Lot, bool) {
	if s.TradeSaleSlot >= 1 && s.TradeSaleSlot <= WarehouseLots {
		lot := WarehouseLotsOf(s, s.CityID)[s.TradeSaleSlot-1]
		return lot, lot.Goods > 0 && lot.Goods < goodsCodes
	}
	if s.TradeSaleSlot >= 5 && s.TradeSaleSlot <= 6 {
		h := s.Holds[s.TradeSaleSlot-5]
		return h.Lot(), h.Kind == HoldGoods && h.Goods > 0 && h.Goods < goodsCodes
	}
	return Lot{}, false
}

func TradeSaleAvailable(s State) bool {
	lot, ok := PendingTradeLot(s)
	if !ok {
		return false
	}
	if s.CityID == world.MissionDestinationID {
		return s.Colony.Initialized && !s.Colony.Complete() && s.Colony.Remaining[lot.Goods] > 0
	}
	return tradeGoodsAccepted(s, lot.Goods)
}

func tradeGoodsAccepted(s State, goods int) bool {
	return goods > 0 && goods < goodsCodes && (s.CityID == world.MissionDestinationID || goods != 1 && goods != 31)
}

func (r Reducer) prepareTradeSale(s *State, slot int) (string, bool) {
	s.TradeSaleSlot, s.TradeSalePrice = slot, 0
	lot, ok := PendingTradeLot(*s)
	if !ok {
		s.TradeSaleSlot = 0
		return "ui.message.no_cargo", false
	}
	if TradeSaleAvailable(*s) {
		price, found := r.CurrentLotPrice(s, lot, true)
		if !found {
			s.TradeSaleSlot = 0
			return "ui.message.no_cargo", false
		}
		s.TradeSalePrice = price
	}
	s.TradePage = TradePageSellConfirm
	return "ui.message.trade_confirm_sale", true
}

func cancelTradeSale(s *State) {
	s.TradeSaleSlot, s.TradeSalePrice = 0, 0
	s.TradePage = TradePageMain
}

func clearTradeSaleSlot(s *State, slot int) {
	if slot >= 1 && slot <= WarehouseLots {
		lots := warehouse(s, s.CityID)
		lots[slot-1] = Lot{}
		s.Warehouses[s.CityID] = lots
	} else if slot >= 5 && slot <= 6 {
		s.Holds[slot-5] = Hold{}
		s.Gems = len(PodLots(*s))
	}
}

// 確認後共用交易分派。價格由已驗證的 CurrentLotPrice caller 提供。
func (r Reducer) settleApprovedLot(s *State, lot Lot, price int) bool {
	if s.CityID != world.MissionDestinationID {
		if lot.Goods == 1 || lot.Goods == 31 || price <= 0 {
			return false
		}
		creditTradeFunds(s, Credits(price))
		r.updateMarket(s, lot.Goods, true)
	} else {
		if price < 0 || s.Colony.Accumulated+price/2 > 32767 || !s.Colony.DeliverLot(lot.Goods) {
			return false
		}
		if lot.Goods == 31 {
			s.BanvilleCargo++
		} else {
			s.BanvilleCargo += lot.Quality
		}
		s.Colony.Accumulated += price / 2
		advanceColonyAtExchange(s)
		if !s.MissionComplete {
			r.cityMarket(s, s.CityID)
			for i := 0; i < QuoteRing; i++ {
				r.addQuote(s, i)
			}
		}
	}
	s.LastSalePrice = price
	return true
}

func (r Reducer) confirmTradeSale(s *State) (string, bool) {
	lot, found := PendingTradeLot(*s)
	if !found || !TradeSaleAvailable(*s) || !r.settleApprovedLot(s, lot, s.TradeSalePrice) {
		return "ui.message.trade_not_needed", false
	}
	if s.Screen != ScreenScore {
		clearTradeSaleSlot(s, s.TradeSaleSlot)
	}
	cancelTradeSale(s)
	if s.Screen == ScreenScore {
		return "ui.message.mission_complete", true
	}
	return "ui.message.sold", true
}

// warehouse is the city's four slots.
func warehouse(state *State, cityID string) []Lot {
	if state.Warehouses == nil {
		state.Warehouses = map[string][]Lot{}
	}
	lots := state.Warehouses[cityID]
	for len(lots) < WarehouseLots {
		lots = append(lots, Lot{})
	}
	state.Warehouses[cityID] = lots
	return lots
}

// WarehouseLotsOf is the city's warehouse for the shells.
func WarehouseLotsOf(state State, cityID string) []Lot {
	lots := append([]Lot(nil), state.Warehouses[cityID]...)
	for len(lots) < WarehouseLots {
		lots = append(lots, Lot{})
	}
	return lots
}

// storeLot moves a pod lot into the first free warehouse slot in this shell;
// fetchLot brings a warehouse slot into the pod.
func (r Reducer) storeLot(state *State, holdIndex int) (string, bool) {
	hold := HoldAt(*state, holdIndex)
	if hold.Kind != HoldGoods || hold.Goods == 0 {
		return "ui.message.no_cargo", false
	}
	slots := warehouse(state, state.CityID)
	free := -1
	for i, slot := range slots {
		if slot.Goods == 0 {
			free = i
			break
		}
	}
	if free < 0 {
		return "ui.message.warehouse_full", false
	}
	slots[free] = hold.Lot()
	state.Warehouses[state.CityID] = slots
	// XDOTRADI:0x10 Store copies a zero two-word record into the source.
	// Other emptyHold callers preserve metadata under their own contracts.
	state.Holds[holdIndex] = Hold{}
	state.Gems = len(PodLots(*state))
	return "ui.message.stored", true
}

func (r Reducer) fetchLot(state *State, slot int) (string, bool) {
	slots := warehouse(state, state.CityID)
	if slot < 0 || slot >= len(slots) || slots[slot].Goods == 0 {
		return "ui.message.warehouse_empty", false
	}
	if CargoLotsFree(*state) <= 0 {
		return "ui.message.cargo_full", false
	}
	kind, _ := r.World.CargoKindByCode(slots[slot].Goods)
	if !addPodLot(state, slots[slot], kind.Key) {
		return "ui.message.cargo_full", false
	}
	slots[slot] = Lot{}
	state.Warehouses[state.CityID] = slots
	return "ui.message.fetched", true
}

// XDOTRADI:10 raw0x44200:0771..07c0 exchanges complete occupied records.
// Unlike Store/Fetch, Swap requires no free slot and never clears either lot.
func (r Reducer) swapLot(state *State, holdIndex, slot int) (string, bool) {
	if holdIndex < 0 || holdIndex >= CargoLots {
		return "ui.message.no_cargo", false
	}
	hold := state.Holds[holdIndex]
	if hold.Kind != HoldGoods || hold.Goods <= 0 || hold.Goods >= goodsCodes {
		return "ui.message.no_cargo", false
	}
	if slot < 0 || slot >= WarehouseLots {
		return "ui.message.warehouse_empty", false
	}
	lots := WarehouseLotsOf(*state, state.CityID)
	incoming := lots[slot]
	kind, ok := r.World.CargoKindByCode(incoming.Goods)
	if incoming.Goods <= 0 || !ok {
		return "ui.message.warehouse_empty", false
	}
	// Reducer input is a value; detach the changed city from its map/slice.
	warehouses := make(map[string][]Lot, len(state.Warehouses)+1)
	for city, stored := range state.Warehouses {
		warehouses[city] = stored
	}
	lots[slot] = hold.Lot()
	warehouses[state.CityID] = lots
	state.Warehouses = warehouses
	state.Holds[holdIndex] = goodsHold(incoming, kind.Key)
	return "ui.message.swapped", true
}

// QualityLetter is how the original prints a quality: 'H' less the figure.
func QualityLetter(quality int) string {
	if quality <= 0 || quality > 7 {
		return "-"
	}
	return string(rune('H' - quality))
}

// MarqueeText is add_quote's entry for a quote: the name, '-', the price,
// padded with dots to a cell of twenty-one.
func (r Reducer) MarqueeText(lot Lot) string {
	if lot.Goods == 0 {
		return ""
	}
	kind, ok := r.World.CargoKindByCode(lot.Goods)
	if !ok {
		return ""
	}
	text := r.CargoNameForKey(kind.Key) + "-" + itoa(lot.Price)
	for len([]rune(text)) < marqueeCell {
		text += "."
	}
	return text
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	digits := []byte{}
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

var _ = world.FacilityExchange
