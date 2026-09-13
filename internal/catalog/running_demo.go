// 跑步演示数据 / Running Demo Catalog
// 功能：幂等提供五个跑步分类、二十件英文商品及中文辅助内容、变体、库存与 SEO 文案
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-11 16:00:00
package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

// RunningDemoResult 记录本次幂等补齐的变化；已有商品只补分类关联，不覆盖商家内容。
type RunningDemoResult struct {
	CategoriesCreated int
	ProductsCreated   int
	LinksCreated      int
}

type runningDemoCategory struct {
	Name, Slug string
	Products   []ProductInput
}

// SeedRunningDemo 显式补齐五个分类和二十件跑步演示商品。整个操作原子提交且可重复执行。
func (s *Service) SeedRunningDemo(ctx context.Context) (RunningDemoResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RunningDemoResult{}, fmt.Errorf("catalog: 开启演示数据事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := s.q.WithTx(tx)
	result := RunningDemoResult{}

	for position, group := range runningDemoCatalog() {
		cat, err := q.GetCategoryBySlug(ctx, group.Slug)
		if errors.Is(err, sql.ErrNoRows) {
			publicID := uuid.Must(uuid.NewV7()).String()
			id, createErr := q.CreateCategory(ctx, sqlcgen.CreateCategoryParams{PublicID: publicID, Name: group.Name, Slug: group.Slug, Position: int64(position)})
			if createErr != nil {
				return RunningDemoResult{}, fmt.Errorf("catalog: 建演示分类失败: %w", createErr)
			}
			cat = sqlcgen.GetCategoryBySlugRow{ID: id, PublicID: publicID, Name: group.Name, Slug: group.Slug, Position: int64(position)}
			result.CategoriesCreated++
		} else if err != nil {
			return RunningDemoResult{}, fmt.Errorf("catalog: 取演示分类失败: %w", err)
		}

		for _, input := range group.Products {
			product, err := q.GetProductBySlug(ctx, input.Slug)
			if errors.Is(err, sql.ErrNoRows) {
				input.CategoryPublicIDs = []string{cat.PublicID}
				if _, createErr := createProducts(ctx, q, []ProductInput{input}); createErr != nil {
					return RunningDemoResult{}, createErr
				}
				result.ProductsCreated++
				result.LinksCreated++
				continue
			}
			if err != nil {
				return RunningDemoResult{}, fmt.Errorf("catalog: 检查演示商品失败: %w", err)
			}
			linkedIDs, err := q.ListCategoryPublicIDsByProduct(ctx, product.ID)
			if err != nil {
				return RunningDemoResult{}, fmt.Errorf("catalog: 检查演示商品分类失败: %w", err)
			}
			alreadyLinked := false
			for _, id := range linkedIDs {
				if id == cat.PublicID {
					alreadyLinked = true
					break
				}
			}
			if !alreadyLinked {
				if err := q.LinkProductCategory(ctx, sqlcgen.LinkProductCategoryParams{ProductID: product.ID, CategoryID: cat.ID}); err != nil {
					return RunningDemoResult{}, fmt.Errorf("catalog: 关联演示分类失败: %w", err)
				}
				result.LinksCreated++
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return RunningDemoResult{}, fmt.Errorf("catalog: 提交演示数据失败: %w", err)
	}
	return result, nil
}

func runningDemoCatalog() []runningDemoCategory {
	return []runningDemoCategory{
		{Name: "Running Tops", Slug: "running-tops", Products: []ProductInput{
			runningProduct("Velocity Air Running Tee", "疾风透气跑步T恤", "velocity-air-running-tee", "疾风透气跑步T恤", "A featherlight running tee with quick-dry comfort for daily miles, tempo sessions and warm-weather training.", "Lightweight quick-dry running T-shirt for breathable everyday training.", 4500, []string{"Cloud White", "Midnight Navy"}),
			runningProduct("Tempo Mesh Singlet", "节奏网眼跑步背心", "tempo-mesh-singlet", "节奏网眼跑步背心", "An airy race-day singlet with open-knit zones that release heat through faster efforts.", "Breathable mesh running singlet for races and speed sessions.", 4200, []string{"Signal Lime", "Midnight Navy"}),
			runningProduct("Horizon Long Sleeve", "地平线跑步长袖", "horizon-running-long-sleeve", "地平线跑步长袖", "A smooth long-sleeve layer for cool starts, shaded trails and relaxed recovery miles.", "Lightweight long-sleeve running top for cooler daily miles.", 5800, []string{"Mist Grey", "Forest Green"}),
			runningProduct("Relay Seamless Tank", "接力无缝训练背心", "relay-seamless-tank", "接力无缝训练背心", "A close, seamless training tank designed to reduce distraction during repeats and gym sessions.", "Seamless running tank for interval and cross-training sessions.", 4800, []string{"Black", "Clay"}),
		}},
		{Name: "Running Bottoms", Slug: "running-bottoms", Products: []ProductInput{
			runningProduct("Pace Split Running Shorts", "配速开衩跑步短裤", "pace-split-running-shorts", "配速开衩跑步短裤", "Easy-moving running shorts with a streamlined feel for fast intervals, steady long runs and recovery days.", "Lightweight split running shorts for unrestricted everyday movement.", 5200, []string{"Midnight Navy", "Forest Green"}),
			runningProduct("Stride Compression Tights", "步频压缩跑步紧身裤", "stride-compression-tights", "步频压缩跑步紧身裤", "Supportive running tights with a smooth, stay-put fit for cooler miles and focused training blocks.", "Supportive compression running tights for cool-weather training.", 7400, []string{"Black", "Graphite"}),
			runningProduct("Trail Pocket Shorts", "越野多袋跑步短裤", "trail-pocket-running-shorts", "越野多袋跑步短裤", "Secure-pocket trail shorts built to carry gels and small essentials without interrupting your stride.", "Multi-pocket trail running shorts for longer off-road sessions.", 6400, []string{"Graphite", "Forest Green"}),
			runningProduct("Recovery Knit Joggers", "恢复针织慢跑裤", "recovery-knit-joggers", "恢复针织慢跑裤", "Soft tapered joggers for warm-ups, cooldowns and the easy hours between training sessions.", "Tapered recovery joggers for warm-ups and everyday comfort.", 7800, []string{"Heather Grey", "Midnight Navy"}),
		}},
		{Name: "Jackets & Layers", Slug: "jackets-and-layers", Products: []ProductInput{
			runningProduct("AeroShield Wind Jacket", "轻风防风跑步夹克", "aeroshield-wind-jacket", "轻风防风跑步夹克", "A packable outer layer designed to take the edge off cool starts and changing weather without weighing you down.", "Packable windproof running jacket for cool and changing conditions.", 8900, []string{"Stone", "Midnight Navy"}),
			runningProduct("Stormline Rain Shell", "风暴线防雨跑步外套", "stormline-rain-shell", "风暴线防雨跑步外套", "A lightweight rain shell with targeted ventilation for wet-weather training and exposed routes.", "Ventilated lightweight running rain shell for wet conditions.", 12900, []string{"Ocean Blue", "Black"}),
			runningProduct("Dawn Thermal Running Vest", "晨曦保暖跑步马甲", "dawn-thermal-running-vest", "晨曦保暖跑步马甲", "Core warmth without bulky sleeves, made for dark winter starts and windy ridge runs.", "Lightweight thermal running vest for cold and windy training.", 9800, []string{"Burnt Orange", "Black"}),
			runningProduct("Packlight Reflective Anorak", "轻装反光套头跑步衣", "packlight-reflective-anorak", "轻装反光套头跑步衣", "A compact reflective anorak that packs into its own pocket when the weather clears.", "Packable reflective running anorak for changing light and weather.", 10900, []string{"Silver", "Signal Lime"}),
		}},
		{Name: "Running Accessories", Slug: "running-accessories", Products: []ProductInput{
			runningAccessory("Daybreak Performance Cap", "破晓轻量跑步帽", "daybreak-performance-cap", "破晓轻量跑步帽", "A breathable, low-profile cap that keeps your focus forward from sunrise runs to weekend distance days.", "Breathable lightweight running cap for sun and sweat management.", 3200, []string{"Cloud White", "Forest Green"}),
			runningAccessory("Flow Hydration Run Belt", "流线补水跑步腰包", "flow-hydration-run-belt", "流线补水跑步腰包", "A bounce-free running belt for carrying small essentials when your route goes farther than planned.", "Bounce-free running belt for phone, keys and small essentials.", 3900, []string{"Black", "Forest Green"}),
			runningAccessory("Nightfall Reflective Socks", "夜跑反光跑步袜", "nightfall-reflective-socks", "夜跑反光跑步袜", "Cushioned running socks with reflective details and breathable zones for low-light miles.", "Reflective cushioned running socks for low-light training.", 1800, []string{"Black", "Cloud White"}),
			runningAccessory("Cadence Running Gloves", "步频轻量跑步手套", "cadence-running-gloves", "步频轻量跑步手套", "Light stretch gloves that take the chill off without trapping excess heat.", "Lightweight stretch running gloves for cool-weather miles.", 2900, []string{"Black", "Graphite"}),
		}},
		{Name: "Hydration & Recovery", Slug: "hydration-and-recovery", Products: []ProductInput{
			runningAccessory("Endurance Handheld Flask", "耐力手持软水壶", "endurance-handheld-flask", "耐力手持软水壶", "A grippy handheld flask that compresses as you drink and stays secure through long runs.", "Soft handheld running flask for convenient mid-run hydration.", 3500, []string{"Clear", "Ocean Blue"}),
			runningAccessory("Ridge Soft Flask 500", "山脊500毫升软水壶", "ridge-soft-flask-500", "山脊500毫升软水壶", "A flexible 500 ml flask shaped for running vests, belts and compact storage after use.", "Flexible 500 ml soft flask for trail and distance running.", 2600, []string{"Clear", "Signal Lime"}),
			runningAccessory("Reset Travel Foam Roller", "重启便携泡沫轴", "reset-travel-foam-roller", "重启便携泡沫轴", "A compact textured roller for post-run mobility at home, the track or while travelling.", "Compact textured foam roller for post-run recovery and mobility.", 4400, []string{"Graphite", "Ocean Blue"}),
			runningAccessory("Cooldown Massage Ball", "冷却恢复按摩球", "cooldown-massage-ball", "冷却恢复按摩球", "A firm portable massage ball for targeted foot, calf and hip recovery after demanding miles.", "Portable massage ball for targeted runner recovery.", 1600, []string{"Signal Lime", "Black"}),
		}},
	}
}

func runningProduct(title, titleZH, slug, slugZH, description, seo string, price int64, colors []string) ProductInput {
	return runningProductWithSizes(title, titleZH, slug, slugZH, description, seo, price, []string{"S", "M", "L"}, colors)
}

func runningAccessory(title, titleZH, slug, slugZH, description, seo string, price int64, colors []string) ProductInput {
	return runningProductWithSizes(title, titleZH, slug, slugZH, description, seo, price, []string{"One Size"}, colors)
}

func runningProductWithSizes(title, titleZH, slug, slugZH, description, seo string, price int64, sizes, colors []string) ProductInput {
	variants := make([]VariantInput, 0, len(sizes)*len(colors))
	for _, size := range sizes {
		for _, color := range colors {
			variants = append(variants, VariantInput{SKU: fmt.Sprintf("RUN-%s-%s-%s", shortSKU(slug), shortSKU(size), shortSKU(color)), PriceCents: price, Quantity: 24, Selections: []Selection{{Option: "Size", Value: size}, {Option: "Color", Value: color}}})
		}
	}
	return ProductInput{
		Title: title, TitleZH: titleZH, Slug: slug, SlugZH: slugZH, Description: description,
		SEODescription: seo, SEODescriptionZH: titleZH + "，适合日常跑步训练与运动穿搭。", Status: "active",
		Options: []OptionInput{{Name: "Size", Values: sizes}, {Name: "Color", Values: colors}}, Variants: variants,
	}
}

func shortSKU(value string) string {
	if len(value) > 5 {
		return value[:5]
	}
	return value
}
