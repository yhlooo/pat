package trading

import (
	"context"
	"fmt"
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
		if typed.MarketSlug != ui.curMarket.Slug {
			ui.curMarket = ui.trader.Market()
		}
		return ui, ui.receiveNextTradingStatus
	}

	return ui, nil
}

// View 渲染显示内容
func (ui *UI) View() string {
	outcomes, _ := ui.curMarket.GetOutcomes()

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
`,
		ui.curMarket.Question, ui.curMarket.Slug,
		ui.curMarket.Description,
		ui.lastStatus.ResolutionSource.URL,
		ui.lastStatus.ResolutionSource.Value,
		ui.curMarket.EndDate.Sub(time.Now()).Round(time.Second).String(),
		outcomes[0],
		ui.lastStatus.Prices.Outcome1.BestBid.StringFixedBank(2),
		ui.lastStatus.Prices.Outcome1.BestAsk.StringFixedBank(2),
		ui.lastStatus.Prices.Outcome1.Last.StringFixedBank(2),
		outcomes[1],
		ui.lastStatus.Prices.Outcome2.BestBid.StringFixedBank(2),
		ui.lastStatus.Prices.Outcome2.BestAsk.StringFixedBank(2),
		ui.lastStatus.Prices.Outcome2.Last.StringFixedBank(2),
	)
}
