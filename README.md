# Homark: UK Housing Market Dynamics & Planning Policy Simulation

**UK housing dynamics & planning policy simulation** built on the [stochadex](https://github.com/umbralcalc/stochadex) SDK: ingest open data, run a single–local-authority monthly model, calibrate, and explore policy-style scenario grids.

This README is written as a **project report**: **outcomes to date**, **limits of what the repo proves**, and **follow-ups**. Deeper stochadex iteration rules and commands live in [`CLAUDE.md`](CLAUDE.md).

---

## 1. Purpose

**Question we care about:** given an area’s prices, earnings, rates, and supply signals, how might **planning- and market-adjacent levers** (approvals, pipeline speed, rate paths, stylised tenure/density knobs) relate to **affordability** (price-to-earnings) in a **transparent, replayable** simulation — not as a substitute for SHMAs or econometric causal claims, but as a **decision-science sandbox**.

---

## 2. Outcomes (what the repository delivers today)

| Area | Outcome |
|------|---------|
| **Data spine** | [`cmd/fetchspine`](cmd/fetchspine/main.go): UK HPI + BoE bank rate → [`dat/processed/spine_monthly.csv`](dat/processed/spine_monthly.csv) for pilot LAs; optional ONS-style earnings & affordability, PPD+NSPL (typed medians D/S/T/F), annual permissions & completions, DLUHC Table 122 net additions. |
| **Pilots** | Nine English LAs in [`pkg/ladata/targets.yaml`](pkg/ladata/targets.yaml): inner north London (Camden, Hackney, Haringey, Islington), Tower Hamlets, St Albans, Leeds, Brighton and Hove, Burnley. |
| **Replay** | [`cmd/runfromspine`](cmd/runfromspine/main.go): historical spine through stochadex storage partitions; optional validation vs ONS ratio. |
| **Forward model** | [`cmd/forwardspine`](cmd/forwardspine/main.go) + [`pkg/housing`](pkg/housing): log earnings & log price (drift–diffusion), affordability `exp(log P − log E)`, bank/supply/pipeline and optional demand–supply pressure; **deterministic** or **stochastic** pipeline. |
| **Calibration** | [`cmd/calibratespine`](cmd/calibratespine/main.go): deterministic **grid** over six coefficients; **Evolution Strategy** (`-es-steps`) with **`theta_mean` / `theta_cov`**; `-validate-months`, `-laplace`, `-es-json-out` for downstream use. |
| **Credibility** | [`cmd/credibilityreport`](cmd/credibilityreport/main.go): spine coverage summary; PPD–HPI correlations; permissions→completions lag scan; optional hold-out grid; `-no-credibility` for calibration-only runs. |
| **Scenarios** | [`cmd/policyscenario`](cmd/policyscenario/main.go): Cartesian grids (approvals × bank scales × optional completion fracs, market fraction, flat-share × composition drift) × optional ES posterior draws. |
| **Plots** | [`cmd/scenarioplot`](cmd/scenarioplot/main.go): CSV → HTML heatmap. [`scripts/run_pilot_scenarios.sh`](scripts/run_pilot_scenarios.sh): batch all pilots → `artifacts/scenarios/`. |
| **Quality** | [`cmd/spinehealth`](cmd/spinehealth/main.go), [`scripts/spinehealth_gate.sh`](scripts/spinehealth_gate.sh): pay/ratio coverage gates. **`go test ./...`** (no hosted CI in-repo). |
| **Worked example** | You can maintain per-LA run reports under `docs/reports/` (e.g. Islington: enrichment → ES → scenarios → credibility) alongside your local `posteriors/*.json` and `artifacts/scenarios/`. |

**Important model note:** On the **deterministic** path, pipeline inflow is the **constant** `-approvals` (× market fraction), **not** the time-varying `permissions_approx_monthly` column; spine permissions drive inflow on the **stochastic** pipeline path (`forwardspine` with `-seed-pipeline`). See [`pkg/housing/forward_spine.go`](pkg/housing/forward_spine.go).

---

## 3. Limitations (honest scope)

- **Single-LA** only — no commuter spillovers, no multi-market system (Phase 5).
- **Scenario numbers are model-internal** until **real** ONS/NOMIS/DLUHC files replace templates, **per-LA calibration** is stable, and **validation** (rolling holdouts, stress periods) is tightened.
- **ES posterior sampling** can be **heavy-tailed**; use **`theta_mean`** or tighter bounds for headline tables until UQ is improved.
- **Credibility** needs **PPD + completions** (and ideally official permissions) on the spine; without them, correlation and lag diagnostics are partial or empty.
- **No rental track**, **no MSOA**, **no developer-strategic** supply — roadmap items only.

---

## 4. Future follow-ups (prioritised)

1. **Production data path** — Official `earnings_annual.csv`, `ons_affordability.csv`, permissions/completions, optional Price Paid + NSPL; then `fetchspine` + **`spinehealth_gate`** at ≥95% pay/ratio per pilot.
2. **Model–data alignment** — Time-varying permissions on the **deterministic** path (or document scenario semantics exclusively under stochastic pipeline); tame ES / posterior behaviour.
3. **Validation** — Rolling holdouts, post-COVID / rate-cycle stress tests; cross-LA checks if multi-LA data pipeline appears.
4. **Phase 5 (pick one thread)** — Multi-LA network, rental (VOA / ONS rent index), MSOA disaggregation, or national scale — each is a large fork; avoid doing all at once.
5. **Product** — Narrative exports from scenario CSVs, optional **CI** (`go test` on push), optional blog/write-up once one LA is **fully real-data** end-to-end.

---

## 5. Quick start

```bash
go test ./...

# Build spine (downloads HPI/BoE unless -skip-download); add dat/raw/*.csv for enrichment
go run ./cmd/fetchspine
go run ./cmd/spinehealth

# One LA
go run ./cmd/calibratespine -la "Leeds" -es-steps 400 -es-json-out posteriors/leeds.json
go run ./cmd/policyscenario -la "Leeds" -posterior posteriors/leeds.json \
  -approvals 0,80,160 -bank-scales 1,1.05 -posterior-samples 0 \
  | go run ./cmd/scenarioplot -out artifacts/scenarios/leeds.html

go run ./cmd/credibilityreport -la "Leeds"
```

Templates and raw-data notes: [`dat/raw/README.md`](dat/raw/README.md). Policy batch: `./scripts/run_pilot_scenarios.sh`. YAML skeleton: [`cfg/single_la_housing.yaml`](cfg/single_la_housing.yaml).

---

## 6. Roadmap vs code (compact)

| Phase | Intent | Status in repo |
|-------|--------|----------------|
| **1 — Data** | HPI, BoE, enrichments, pilots | **Done** for pipeline + optional series; **your machine** supplies official CSVs for serious runs. |
| **2 — Model** | Forward/replay, pipeline, demand–supply term | **Done** single-LA monthly. |
| **3 — Learning** | Grid + ES + credibility | **Done** for single-LA; fitted delay **distributions** not done. |
| **4 — Scenarios** | Grids, plots, batch | **MVP done**; narrative / dashboards **follow-up**. |
| **5 — Extensions** | Multi-LA, rental, MSOA, … | **Not implemented**. |

---

## 7. Why this problem (context)

Affordability has shifted structurally since the 1990s; net supply remains below political targets; permissions often do not convert cleanly to homes; and LA decisions rarely sit inside a **repeatable quantitative** counterfactual. Homark is an **engineering** attempt to chain **open data → simulation → calibration → scenario grid** in one repo, with uncertainty hooks (ES) where we can afford them.

**Gap vs other tools:** hedonic models don’t forward-simulate policy paths; SHMAs are often thin on dynamics and uncertainty; national models don’t resolve LA planning. Homark targets **LA-monthly** trajectories with **stochastic** tooling, accepting that **validation** must catch up to **ambition**.

---

## 8. Research background (sketch)

**Intended state (longer horizon):** coupled price process, supply pipeline with delay and attrition, demand drivers, affordability and composition metrics, ensembles over rate scenarios.

**In code today:** a **reduced** realisation — scalar log price & earnings, pipeline stock feeding drift when coefficients are nonzero, affordability as P/E, optional stochastic completions/attrition.

**Policy dimensions in the research vision** include volume, density, tenure, location, build-out rules; **in code today** those are partially approximated by **approval rate**, **completion fracs**, **market delivery fraction**, and **composition drift** (flat-share × beta), not a full tenure accounting model.

---

## 9. Key data sources (reference)

| Source | Role |
|--------|------|
| [UK HPI](https://landregistry.data.gov.uk) | LA monthly prices/indices |
| [BoE Bank Rate](https://www.bankofengland.co.uk/statistics) | Monthly mean rate on spine |
| [Price Paid](https://www.gov.uk/government/statistical-data-sets/price-paid-data-downloads) | Transactions → PPD medians (+ NSPL → LA) |
| ONS affordability & ASHE / NOMIS-style earnings | Ratio & pay on spine |
| DLUHC live tables / open data | Net additions, permissions, completions |
| See also table in git history or expand from [gov.uk](https://www.gov.uk) / [ONS](https://www.ons.gov.uk) as needed |

---

## 10. References & related work

- ONS Housing Affordability (England and Wales) — LA and MSOA affordability statistics.
- MHCLG / DLUHC housing supply releases — net additions, permissions, completions.
- Land Registry Price Paid — longitudinal transactions.
- DLUHC planning statistics — permission-to-delivery framing.

---

*Repository: [`github.com/umbralcalc/homark`](https://github.com/umbralcalc/homark). For stochadex iteration conventions and extra CLI examples, see [`CLAUDE.md`](CLAUDE.md).*
