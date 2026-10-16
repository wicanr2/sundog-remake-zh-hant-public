package game

import "testing"

// 此測試連接商店與工程艙的正式動作；完整船況與場景切換是測試前置，
// 不作為正常 GUI 玩家路徑或原版同狀態證據。
func TestPurchasedScannerManualSwapEnablesLandingGate(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	s := newGameWithFuel()
	step := func(action Action) {
		t.Helper()
		s, err = r.Step(s, action)
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
	}
	step(ActionPlay)
	step(ActionOpenGround)
	step(ActionOpenParts)
	if s.Screen != ScreenParts {
		t.Fatalf("零件櫃檯未開啟：%s", s.MessageKey)
	}
	source, ok := sourceShipSystem(ShipSystemPilotage)
	if !ok || source.Alternate == nil {
		t.Fatal("缺少掃描器來源")
	}
	scanner := source.Alternate.ItemIndex
	found := false
	for range PartsForSale() {
		part, valid := s.SelectedPart()
		if valid && part == scanner {
			found = true
			break
		}
		step(ActionNextPart)
	}
	if !found {
		t.Fatal("商店找不到掃描器")
	}
	price, ok := r.ShopItemPrice(s, scanner)
	if !ok {
		t.Fatal("掃描器沒有價格")
	}
	money := s.Bank + s.Cash
	step(ActionBuyPart)
	if s.MessageKey != "ui.message.part_bought" || s.Bank+s.Cash != money-price {
		t.Fatal("購買或扣款失敗")
	}
	if GroundScannerFitted(s) {
		t.Fatal("只有攜帶就被視為裝妥")
	}
	s.Screen = ScreenEngineering
	for i, system := range s.ShipSystems {
		if system.ID == ShipSystemPilotage {
			s.SelectedSystemIndex = i
		}
	}
	for i, part := range s.Parts {
		if part == scanner {
			s = r.PickPocketPart(s, i)
			break
		}
	}
	if s.HandItem != scanner {
		t.Fatal("無法拿起已購買的掃描器")
	}
	column := source.Alternate.Column
	old := s.ShipSystems[s.SelectedSystemIndex].Rows[0][column]
	s, ok = r.MoveHandToSlot(s, 0, column)
	if !ok || !GroundScannerFitted(s) || s.HandItem != old {
		t.Fatal("手動交換沒有啟用掃描器或遺失舊零件")
	}
	s = r.DropHandIntoPocket(s)
	if s.HandItem != 0 {
		t.Fatal("舊零件無法收回口袋")
	}
	// 拆下掃描器後應立即撤銷降落能力，不能靠過期旗標通過。
	s, ok = r.MoveHandToSlot(s, 0, column)
	if !ok || s.HandItem != scanner || GroundScannerFitted(s) {
		t.Fatal("拆卸後掃描器門檻沒有恢復")
	}
}
