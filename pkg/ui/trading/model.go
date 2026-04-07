package trading

import (
	"context"
	"fmt"
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yhlooo/pat/pkg/polymarket"
	"github.com/yhlooo/pat/pkg/trading"
)

// NewUI 创建 UI
func NewUI(trader trading.Trader) *UI {
	return &UI{trader: trader}
}

// UI 交易交互界面
type UI struct {
	trader trading.Trader

	lastStatus trading.Status
	curMarket  polymarket.Market
}

var _ tea.Model = (*UI)(nil)

// Run 开始运行
func (ui *UI) Run(ctx context.Context) error {
	if err := ui.trader.Start(ctx); err != nil {
		return fmt.Errorf("start trader error: %w", err)
	}
	ui.curMarket = ui.trader.Market()

	p := tea.NewProgram(ui, tea.WithContext(ctx))
	_, err := p.Run()
	return err
}

// Init 返回第一个操作
func (ui *UI) Init() tea.Cmd {
	return tea.Batch(
		ui.receiveNextTradingStatus,
		tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
			return TickMsg(t)
		}),
	)
}

// receiveNextTradingStatus 接收下一个交易状态
func (ui *UI) receiveNextTradingStatus() tea.Msg {
	status, ok := <-ui.trader.Channel()
	if !ok {
		return tea.Quit()
	}
	return status
}

type TickMsg time.Time

// Update 处理更新事件
func (ui *UI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.KeyMsg:
		switch typed.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return ui, tea.Quit
		}
	case TickMsg:
		return ui, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
			return TickMsg(t)
		})
	case trading.Status:
		ui.lastStatus = typed
		if typed.CurrentMarket.Slug != ui.curMarket.Slug {
			ui.curMarket = ui.trader.Market()
		}
		return ui, ui.receiveNextTradingStatus
	}

	return ui, nil
}

// View 渲染显示内容
func (ui *UI) View() string {
	yes, no, _ := ui.curMarket.GetOutcomes()

	holding := ""
	for _, a := range ui.lastStatus.GetHoldingList() {
		holding += fmt.Sprintf(
			"  - %s %s %s worth %s USD\n",
			a.MarketSlug, a.Type, a.Quantity.Round(2).String(), a.Value.Round(4).String(),
		)
	}

	pendingOrderList := ui.lastStatus.GetPendingOrderList()
	slices.Reverse(pendingOrderList)
	pendingOrders := ""
	for _, order := range pendingOrderList {
		qty := order.Quantity.Round(2).String()
		if (order.Type == trading.FAK || order.Type == trading.FOK) && order.Side == trading.Buy {
			qty = order.Amount.Round(2).String() + " USD"
		}
		price := "Bid " + order.Price.Round(2).String()
		if order.Side == trading.Sell {
			price = "Ask " + order.Price.Round(2).String()
		}
		if order.Type == trading.FAK || order.Type == trading.FOK {
			price = ""
		}
		pendingOrders += fmt.Sprintf(
			"  - %s %s %s %s %s %s %s\n",
			order.CreatedAt.Format(time.TimeOnly), order.Type, order.Side, qty, order.TokenType, order.MarketSlug, price,
		)
	}

	completedOrderList := ui.lastStatus.GetCompletedOrderList()
	slices.Reverse(completedOrderList)
	if len(completedOrderList) > 10 {
		completedOrderList = completedOrderList[:10]
	}
	completedOrders := ""
	for _, order := range completedOrderList {
		switch order.State {
		case trading.OrderFilled:
			completedOrders += fmt.Sprintf(
				"  - %s %s %s %s filled %s at %s USD (avg price: %s)\n",
				order.ResolvedAt.Format(time.TimeOnly), order.Side, order.MarketSlug, order.TokenType,
				order.FilledQuantity.Round(2).String(),
				order.FilledAmount.Round(2).String(),
				order.FilledPrice.Round(2).String(),
			)
		case trading.OrderFailed, trading.OrderCancelled:
			completedOrders += fmt.Sprintf(
				"  - %s %s %s %s %s\n",
				order.ResolvedAt.Format(time.TimeOnly), order.Side, order.MarketSlug, order.TokenType, order.State,
			)
		}
	}

	return fmt.Sprintf(`# %s (%s)

%s

Resolution Source: %s
- Value: %s

Timer: %s

- %s
  - best bid: %s
  - best ask: %s
  - last: %s
- %s
  - best bid: %s 
  - best ask: %s
  - last: %s

## Assets

- Cash: %s
- Holding:
%s
## Orders

- Pending:
%s
- Completed:
%s
`,
		ui.curMarket.Question, ui.curMarket.Slug,
		ui.curMarket.Description,
		ui.lastStatus.ResolutionSource.URL,
		ui.lastStatus.ResolutionSource.Value,
		ui.curMarket.EndDate.Sub(time.Now()).Round(time.Second).String(),
		yes,
		ui.lastStatus.Prices.Yes.BestBid.StringFixedBank(2),
		ui.lastStatus.Prices.Yes.BestAsk.StringFixedBank(2),
		ui.lastStatus.Prices.Yes.Last.StringFixedBank(2),
		no,
		ui.lastStatus.Prices.No.BestBid.StringFixedBank(2),
		ui.lastStatus.Prices.No.BestAsk.StringFixedBank(2),
		ui.lastStatus.Prices.No.Last.StringFixedBank(2),
		ui.lastStatus.Cash.StringFixedBank(2),
		holding,
		pendingOrders,
		completedOrders,
	)
}
