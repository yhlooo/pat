package main

import (
	"flag"
	"fmt"
	"image/color"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
)

// 模型参数
type Params struct {
	S0        float64 // 初始价格
	Sigma     float64 // 波动率（年化）
	TimeYears float64 // 总时长（年）
	Steps     int     // 模拟步数
	JumpProb  float64 // 跳跃概率（每年）
	JumpMean  float64 // 跳跃均值（对数）
	JumpStd   float64 // 跳跃标准差（对数）
}

// 生成算术随机游走路径
func arithmeticRandomWalk(p Params, rng *rand.Rand) plotter.XYs {
	dt := p.TimeYears / float64(p.Steps)
	sigmaDt := p.Sigma * math.Sqrt(dt) // 每步绝对波动幅度
	path := make(plotter.XYs, p.Steps+1)
	path[0].X = 0
	path[0].Y = p.S0

	S := p.S0
	for i := 1; i <= p.Steps; i++ {
		// 50% 向上 +sigmaDt，50% 向下 -sigmaDt
		if rng.Float64() < 0.5 {
			S += sigmaDt
		} else {
			S -= sigmaDt
		}
		path[i].X = float64(i) * dt
		path[i].Y = S
	}
	return path
}

// 生成几何布朗运动路径（连续复利）
func geometricBrownianMotion(p Params, rng *rand.Rand) plotter.XYs {
	dt := p.TimeYears / float64(p.Steps)
	// 对数收益率的漂移和扩散
	mu := 0.0 // 假设无风险利率为0，仅关注波动
	drift := (mu - 0.5*p.Sigma*p.Sigma) * dt
	vol := p.Sigma * math.Sqrt(dt)

	path := make(plotter.XYs, p.Steps+1)
	path[0].X = 0
	path[0].Y = p.S0

	S := p.S0
	for i := 1; i <= p.Steps; i++ {
		z := rng.NormFloat64()
		logReturn := drift + vol*z
		S *= math.Exp(logReturn)
		path[i].X = float64(i) * dt
		path[i].Y = S
	}
	return path
}

// 生成带跳跃的几何布朗运动路径（Merton模型）
func geometricBrownianMotionWithJumps(p Params, rng *rand.Rand) plotter.XYs {
	dt := p.TimeYears / float64(p.Steps)
	mu := 0.0
	// 跳跃补偿项：使期望收益不变（风险中性下）
	jumpCompensation := p.JumpProb * (math.Exp(p.JumpMean+0.5*p.JumpStd*p.JumpStd) - 1)
	drift := (mu - 0.5*p.Sigma*p.Sigma - jumpCompensation) * dt
	vol := p.Sigma * math.Sqrt(dt)

	lambdaDt := p.JumpProb * dt // 每个时间步发生跳跃的概率

	path := make(plotter.XYs, p.Steps+1)
	path[0].X = 0
	path[0].Y = p.S0

	S := p.S0
	for i := 1; i <= p.Steps; i++ {
		// 扩散部分
		z := rng.NormFloat64()
		logReturn := drift + vol*z

		// 跳跃部分
		if rng.Float64() < lambdaDt {
			// 跳跃幅度服从对数正态分布
			jumpSize := math.Exp(p.JumpMean + p.JumpStd*rng.NormFloat64())
			logReturn += math.Log(jumpSize)
		}

		S *= math.Exp(logReturn)
		path[i].X = float64(i) * dt
		path[i].Y = S
	}
	return path
}

func main() {
	// 命令行参数
	S0 := flag.Float64("s0", 100.0, "初始价格")
	sigma := flag.Float64("sigma", 0.2, "年化波动率 (例如 0.2 表示20%)")
	days := flag.Float64("days", 1.0, "模拟时长（天）")
	stepsPerDay := flag.Int("steps", 390, "每天步数 (默认390对应分钟)")
	jumpProb := flag.Float64("jumpprob", 2.0, "年化跳跃强度 (每年平均跳跃次数)")
	jumpMean := flag.Float64("jumpmean", -0.05, "跳跃幅度的对数均值")
	jumpStd := flag.Float64("jumpstd", 0.1, "跳跃幅度的对数标准差")
	seed := flag.Int64("seed", time.Now().UnixNano(), "随机种子")
	output := flag.String("o", ".temp/rand-k-line.png", "输出文件路径")
	flag.Parse()

	// 计算总步数和时间（以年为单位）
	timeYears := *days / 252.0 // 一年252个交易日
	totalSteps := int(math.Round(*days * float64(*stepsPerDay)))

	params := Params{
		S0:        *S0,
		Sigma:     *sigma,
		TimeYears: timeYears,
		Steps:     totalSteps,
		JumpProb:  *jumpProb,
		JumpMean:  *jumpMean,
		JumpStd:   *jumpStd,
	}

	fmt.Printf("模拟参数: 初始价=%.2f, 波动率=%.2f%%, 天数=%.1f, 步数=%d, 跳跃强度=%.2f/年\n",
		params.S0, params.Sigma*100, *days, params.Steps, params.JumpProb)

	// 创建随机数生成器
	rng := rand.New(rand.NewSource(*seed))

	// 生成三条路径
	pathArith := arithmeticRandomWalk(params, rng)
	pathGBM := geometricBrownianMotion(params, rng)
	pathJump := geometricBrownianMotionWithJumps(params, rng)

	// 创建绘图
	p := plot.New()
	p.Title.Text = "随机波动模型对比"
	p.X.Label.Text = "时间 (年)"
	p.Y.Label.Text = "价格"

	// 添加三条线
	addLine := func(data plotter.XYs, name string, col color.Color) {
		line, err := plotter.NewLine(data)
		if err != nil {
			log.Fatal(err)
		}
		line.Color = col
		p.Add(line)
		p.Legend.Add(name, line)
	}

	addLine(pathArith, "算术随机游走", color.RGBA{R: 255, G: 0, B: 0, A: 255})        // 红色
	addLine(pathGBM, "几何布朗运动", color.RGBA{R: 0, G: 0, B: 255, A: 255})          // 蓝色
	addLine(pathJump, "带跳跃的几何布朗运动", color.RGBA{R: 0, G: 128, B: 0, A: 255}) // 绿色

	p.Legend.Top = true
	p.Legend.Left = true

	// 保存图片
	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := p.Save(8*vg.Inch, 6*vg.Inch, *output); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("图表已保存为 %s\n", *output)
}
