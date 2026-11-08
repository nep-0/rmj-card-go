package main

import (
	"fmt"
	"math"
	"strings"

	"github.com/nep-0/rmj"
)

const (
	cardWidth  = 1280
	cardHeight = 1100
	navy       = "#1d4ed8"
	lime       = "#a3e635"
	amber      = "#fbbf24"
	red        = "#f87171"
)

func renderCard(p rmj.PlayerStats, style, club string) string {
	if club == "" {
		club = "雀庄未知"
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, cardWidth, cardHeight, cardWidth, cardHeight)
	b.WriteString(`<style>text{font-family:Arial,"Noto Sans CJK SC",sans-serif}.card{fill:#fff;stroke:#e4eaf4}.name,.value,.rank{fill:#1e3a8a;font-weight:700}.name{font-size:36px}.club{font-size:17px;fill:#55708f}.metric{font-size:18px;fill:#27364a}.value{font-size:31px;text-anchor:middle}.label{font-size:16px;fill:#5c6c80;text-anchor:middle}.small{font-size:13px;fill:#7b8797;text-anchor:middle}.rank{font-size:41px;text-anchor:middle}.rate{font-size:22px;fill:#27364a;text-anchor:middle}.section{font-size:21px;font-weight:700;fill:#15243f;text-anchor:middle}.promo{font-size:17px;fill:#fff;text-anchor:middle}.recent{font-size:17px;fill:#0f766e;text-anchor:middle}.count{font-size:20px;font-weight:700;fill:#15243f;text-anchor:middle}</style><rect width="1280" height="1100" fill="#f5f7fb"/>`)
	b.WriteString(`<rect class="card" x="20" y="20" width="350" height="220" rx="8"/><rect class="card" x="390" y="20" width="430" height="220" rx="8"/><rect class="card" x="840" y="20" width="420" height="220" rx="8"/><circle cx="92" cy="130" r="58" fill="#dbe4f3"/>`)
	fmt.Fprintf(&b, `<text x="174" y="77" class="name">%s</text><text x="174" y="113" class="club">%s</text><text x="174" y="150" class="metric">全国排名 #%d</text><text x="174" y="183" class="metric">雀庄排名 #%d</text><text x="174" y="216" class="metric">总局数 %d</text>`, escape(p.Name), escape(club), p.NationalRank, p.MahjongRank, p.TotalGames)
	grade, hasGrade := rmj.GradeByNumber(p.Grade)
	avgReq, sumReq := "（最高段位）", "（最高段位）"
	if hasGrade && grade.PromotionGameTarget > 0 {
		avgReq = fmt.Sprintf("（≤%.1f 可升段）", grade.PromotionAverageMax)
		sumReq = fmt.Sprintf("（≤%d 可升段）", grade.PromotionSumMax)
	}
	fmt.Fprintf(&b, `<text x="500" y="83" class="value">%d</text><text x="500" y="112" class="label">最高点数</text><text x="710" y="83" class="value">%.2f</text><text x="710" y="112" class="label">平均点数</text><text x="500" y="168" class="value">%.2f</text><text x="500" y="197" class="label">当前均顺</text><text x="500" y="218" class="small">%s</text><text x="710" y="168" class="value">%d</text><text x="710" y="197" class="label">顺位之和</text><text x="710" y="218" class="small">%s</text>`, p.HighestPoints, p.AveragePoints, averagePlace(p), avgReq, p.PromotionPositionSum, sumReq)
	fmt.Fprintf(&b, `<text x="1050" y="113" class="rank">%s</text><text x="1050" y="154" class="rate">RATE #%d</text>`, escape(gradeName(p.Grade)), p.Rating)
	b.WriteString(`<rect class="card" x="20" y="260" width="1240" height="430" rx="8"/><text x="640" y="295" class="section">最近顺位数据（旧 → 新）</text>`)
	if style == "QH" {
		renderTiles(&b, p)
	} else {
		renderTrend(&b, p)
	}
	renderRecentSummary(&b, p)
	if !hasGrade || grade.PromotionGameTarget == 0 {
		b.WriteString(`<rect x="20" y="585" width="1240" height="78" rx="8" fill="#1d4ed8"/><text x="640" y="632" class="promo">已达到最高段位</text>`)
	} else {
		fast, loose := promotion(p)
		if fast.games == 0 {
			fmt.Fprintf(&b, `<rect x="20" y="585" width="1240" height="78" rx="8" fill="#1d4ed8"/><text x="640" y="632" class="promo">最近 %d/%d 局：无法推导升段条件</text>`, len(p.RecentPositions), grade.PromotionGameTarget)
		} else {
			fmt.Fprintf(&b, `<rect x="20" y="585" width="1240" height="78" rx="8" fill="#1d4ed8"/><text x="640" y="622" class="promo">当前最快的升段条件是：%d 半庄顺位之和 ≤ %d</text><text x="640" y="650" class="promo">当前最宽松升段条件是：%d 半庄顺位之和 ≤ %d</text>`, fast.games, fast.sum, loose.games, loose.sum)
		}
	}
	b.WriteString(`<rect class="card" x="20" y="710" width="590" height="370" rx="8"/><text x="315" y="745" class="section">能力分析</text><rect class="card" x="630" y="710" width="630" height="370" rx="8"/><text x="945" y="745" class="section">顺位分布</text><line x1="670" y1="750" x2="1215" y2="750" stroke="#ddd"/>`)
	renderRadar(&b, p)
	renderPlacements(&b, p)
	b.WriteString(`</svg>`)
	return b.String()
}

type promotionPath struct{ games, sum int }

func promotion(p rmj.PlayerStats) (promotionPath, promotionPath) {
	grade, ok := rmj.GradeByNumber(p.Grade)
	if !ok || grade.PromotionGameTarget == 0 || p.PromotionAveragePlace <= 0 {
		return promotionPath{}, promotionPath{}
	}
	gamesPlayed := int(math.Round(float64(p.PromotionPositionSum) / p.PromotionAveragePlace))
	history := recentPositions(p)
	history = history[len(history)-min(len(history), gamesPlayed, grade.PromotionGameTarget):]
	minimumGames := max(1, grade.PromotionGameTarget-len(history))
	var candidates []promotionPath
	for games := minimumGames; games <= grade.PromotionGameTarget; games++ {
		retained := max(0, grade.PromotionGameTarget-games)
		retainedSum := 0
		if retained > 0 {
			for _, placement := range history[len(history)-retained:] {
				retainedSum += placement
			}
		}
		requiredSum := grade.PromotionSumMax - retainedSum
		if requiredSum >= games && requiredSum <= 4*games {
			candidates = append(candidates, promotionPath{games: games, sum: requiredSum})
		}
	}
	if len(candidates) == 0 {
		return promotionPath{}, promotionPath{}
	}
	loosest := candidates[0]
	for _, candidate := range candidates[1:] {
		if float64(candidate.sum)/float64(candidate.games) > float64(loosest.sum)/float64(loosest.games) {
			loosest = candidate
		}
	}
	return candidates[0], loosest
}

func renderTiles(b *strings.Builder, p rmj.PlayerStats) {
	positions := recentPositions(p)
	points := recentPoints(p)
	count := min(10, len(positions), len(points))
	if count == 0 {
		return
	}
	positions = positions[len(positions)-count:]
	points = points[len(points)-count:]
	width, gap := 94, 18
	left := (cardWidth - count*width - (count-1)*gap) / 2
	fills := []string{"#e5e9f5", "#eaf5e4", "#fff6dd", "#fbe8e8"}
	strokes := []string{"#c7d1ea", "#d1e7c4", "#f5eac8", "#f3cccc"}
	colors := []string{"#315aa4", "#67ad56", "#e9ae2f", "#e36363"}
	for i := range count {
		place := positions[i]
		if place < 1 || place > 4 {
			continue
		}
		x := left + i*(width+gap)
		fmt.Fprintf(b, `<rect x="%d" y="330" width="94" height="58" rx="10" fill="%s" stroke="%s" stroke-width="2"/><text x="%d" y="370" text-anchor="middle" font-size="32" font-weight="700" fill="%s">%d</text><rect x="%d" y="417" width="94" height="58" rx="10" fill="#fff" stroke="#e5e7eb" stroke-width="2"/><text x="%d" y="457" text-anchor="middle" font-size="26" font-weight="700" fill="#192337">%d</text>`, x, fills[place-1], strokes[place-1], x+47, colors[place-1], place, x, x+47, points[i])
	}
}

func renderTrend(b *strings.Builder, p rmj.PlayerStats) {
	placements := recentPositions(p)
	count := min(50, len(placements))
	if count < 2 {
		return
	}
	placements = placements[len(placements)-count:]
	fast, loose := promotion(p)
	forecastCount := max(fast.games, loose.games)
	width, height := 850.0, 100.0
	x0, y0 := (float64(cardWidth)-width)/2, 350.0
	endX := x0 + width
	for _, level := range []float64{0, 0.5, 1} {
		y := y0 + height*level
		fmt.Fprintf(b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#dbe4f3"/>`, x0, y, endX, y)
	}
	pointX := func(index int) float64 { return x0 + (endX-x0)*float64(index)/float64(count-1+forecastCount) }
	pointY := func(place float64) float64 { return y0 + height*(place-1)/3 }
	points := make([]string, 0, count)
	for i, place := range placements {
		points = append(points, fmt.Sprintf("%.1f,%.1f", pointX(i), pointY(float64(place))))
	}
	fmt.Fprintf(b, `<polyline points="%s" fill="none" stroke="#3b63f6" stroke-width="3"/>`, strings.Join(points, " "))
	for _, point := range points {
		x, y, _ := strings.Cut(point, ",")
		fmt.Fprintf(b, `<circle cx="%s" cy="%s" r="2.5" fill="#1d4ed8"/>`, x, y)
	}
	latestX, latestY := pointX(count-1), pointY(float64(placements[count-1]))
	for _, path := range []struct {
		promotionPath
		color, label string
	}{{fast, "#16a34a", "快速"}, {loose, "#dc2626", "宽松"}} {
		if path.games == 0 {
			continue
		}
		endIndex := count - 1 + path.games
		targetX := pointX(endIndex)
		targetY := pointY(float64(path.sum) / float64(path.games))
		fmt.Fprintf(b, `<polyline points="%.1f,%.1f %.1f,%.1f %.1f,%.1f" fill="none" stroke="%s" stroke-width="2" stroke-dasharray="7 7"/><text x="%.1f" y="%.1f" font-size="13" fill="%s">%s</text>`, latestX, latestY, pointX(count), targetY, targetX, targetY, path.color, targetX+5, targetY+4, path.color, path.label)
	}
}

func renderRecentSummary(b *strings.Builder, p rmj.PlayerStats) {
	placements := recentPositions(p)
	count := 0
	if p.PromotionAveragePlace > 0 {
		count = min(50, max(0, int(math.Round(float64(p.PromotionPositionSum)/p.PromotionAveragePlace))), len(placements))
	}
	if count == 0 {
		b.WriteString(`<text x="640" y="545" class="recent-empty" text-anchor="middle">暂无最近对局数据</text>`)
		return
	}
	placements = placements[len(placements)-count:]
	groups := (count + 4) / 5
	groupWidth := 104
	left := 640 - groups*groupWidth/2
	fmt.Fprintf(b, `<text x="%d" y="535" class="label">场次</text><text x="%d" y="562" class="label">顺位</text>`, left-42, left-42)
	for group := range groups {
		start := group * 5
		end := min(start+5, count)
		x := left + group*groupWidth + groupWidth/2
		var sequence strings.Builder
		for _, place := range placements[start:end] {
			sequence.WriteByte(byte('0' + place))
		}
		fmt.Fprintf(b, `<text x="%d" y="535" class="recent">%d-%d</text><text x="%d" y="562" class="count">%s</text>`, x, start+1, end, x, sequence.String())
	}
}

func recentPositions(p rmj.PlayerStats) []int {
	positions := make([]int, 0, len(p.RecentPositions))
	for i := len(p.RecentPositions) - 1; i >= 0; i-- {
		placement := p.RecentPositions[i]
		if placement >= 1 && placement <= 4 {
			positions = append(positions, placement)
		}
	}
	return positions
}

func recentPoints(p rmj.PlayerStats) []int {
	points := make([]int, len(p.RecentPoints))
	for i := range p.RecentPoints {
		points[i] = p.RecentPoints[len(p.RecentPoints)-1-i]
	}
	return points
}

func renderRadar(b *strings.Builder, p rmj.PlayerStats) {
	values := []float64{clamp(-p.Fire + 3), clamp(0.000125*p.Attack + 0.375), clamp(0.025*p.Technique + 0.5), clamp(10*p.Luck - 2), clamp(5*p.Stability - 2), clamp(p.Defense/0.3 - 2)}
	labels := []string{"火力", "进攻", "技术", "运势", "稳定", "防守"}
	cx, cy, radius := 315.0, 920.0, 130.0
	for level := 1; level <= 5; level++ {
		var pts []string
		for i := range 6 {
			angle := math.Pi/2 + 2*math.Pi*float64(i)/6
			r := radius * float64(level) / 5
			pts = append(pts, fmt.Sprintf("%.1f,%.1f", cx+r*math.Cos(angle), cy-r*math.Sin(angle)))
		}
		fmt.Fprintf(b, `<polygon points="%s" fill="none" stroke="#e5e7eb"/>`, strings.Join(pts, " "))
	}
	var pts []string
	for i, value := range values {
		angle := math.Pi/2 + 2*math.Pi*float64(i)/6
		pts = append(pts, fmt.Sprintf("%.1f,%.1f", cx+radius*value*math.Cos(angle), cy-radius*value*math.Sin(angle)))
	}
	fmt.Fprintf(b, `<polygon points="%s" fill="#1d4ed8" fill-opacity=".25" stroke="#1d4ed8" stroke-width="2"/>`, strings.Join(pts, " "))
	for i, label := range labels {
		angle := math.Pi/2 + 2*math.Pi*float64(i)/6
		x := cx + (radius+25)*math.Cos(angle)
		y := cy - (radius+25)*math.Sin(angle) + 5
		fmt.Fprintf(b, `<text x="%.1f" y="%.1f" text-anchor="middle" font-size="11" fill="#aaa">%s</text>`, x, y, label)
	}
}

func renderPlacements(b *strings.Builder, p rmj.PlayerStats) {
	counts := []int{p.FirstPlaces, p.SecondPlaces, p.ThirdPlaces, p.FourthPlaces}
	colors := []string{navy, lime, amber, red}
	labels := []string{"一位", "二位", "三位", "四位"}
	total := 0
	for _, count := range counts {
		total += count
	}
	cx, cy, r, inner := 940.0, 910.0, 148.0, 84.0
	angle := -math.Pi / 2
	for i, count := range counts {
		fraction := 0.0
		if total > 0 {
			fraction = float64(count) / float64(total)
		}
		end := angle + 2*math.Pi*fraction
		p1x, p1y := cx+r*math.Cos(angle), cy+r*math.Sin(angle)
		p2x, p2y := cx+r*math.Cos(end), cy+r*math.Sin(end)
		q1x, q1y := cx+inner*math.Cos(end), cy+inner*math.Sin(end)
		q2x, q2y := cx+inner*math.Cos(angle), cy+inner*math.Sin(angle)
		large := 0
		if fraction > .5 {
			large = 1
		}
		fmt.Fprintf(b, `<path d="M %.2f %.2f A %g %g 0 %d 1 %.2f %.2f L %.2f %.2f A %g %g 0 %d 0 %.2f %.2f Z" fill="%s" stroke="#fff" stroke-width="2"/>`, p1x, p1y, r, r, large, p2x, p2y, q1x, q1y, inner, inner, large, q2x, q2y, colors[i])
		percent := percentage(count, total)
		fmt.Fprintf(b, `<circle cx="1120" cy="%d" r="10" fill="%s"/><text x="1140" y="%d" font-size="12">%s · %.2f%%</text>`, 818+i*48, colors[i], 824+i*48, labels[i], percent)
		angle = end
	}
}

func clamp(value float64) float64 { return math.Max(0, math.Min(1, value)) }
