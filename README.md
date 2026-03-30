# UK Housing Market Dynamics & Planning Policy Simulation: Project Plan

## Applying the Stochadex to Housing Supply Strategy Optimisation

---

## Overview

Build a stochastic simulation of local housing market dynamics — price movements, supply delivery, affordability evolution — learned from freely available transaction, earnings, and housebuilding data, with a decision science layer to evaluate planning policy strategies for local authorities.

The core question: **given a local authority's current housing stock, price dynamics, demographic trajectory and economic conditions, what mix of planning approvals (density, tenure, location, timing) minimises the expected affordability gap over 10–20 years?**

---

## Implementation status (repository)

What exists in this codebase today:

- **Pilot local authorities:** five English LAs with ONS GSS codes in [`pkg/ladata/targets.yaml`](pkg/ladata/targets.yaml) (Tower Hamlets, St Albans, Leeds, Brighton and Hove, Burnley).
- **Monthly data spine:** [`cmd/fetchspine`](cmd/fetchspine/main.go) downloads the UK HPI full CSV, the BoE Official Bank Rate (IUDBEDR), and (by default) **DLUHC Live Table 122** (net additional dwellings, `.ods`), then writes [`dat/processed/spine_monthly.csv`](dat/processed/spine_monthly.csv) restricted to those LAs. Bank rate is the **monthly mean** of daily observations joined on `YYYY-MM`. The spine always includes columns for optional enrichments (empty when data is absent): `median_ratio` (ONS affordability CSV), `net_additional_dwellings_fy` (Table 122, joined by UK financial year start for each calendar month), `median_gross_annual_pay` (`dat/raw/earnings_annual.csv` or `-earnings`: `area_code`, `year`, `median_gross_annual_pay`), and `ppd_median_price` / `ppd_sales_count` when both `dat/raw/price_paid.csv` and `dat/raw/nspl.csv` (postcode → LA) are present (or `-ppd` / `-nspl`). Raw downloads go under `dat/raw/` (ignored by git). Use `-skip-download` to rebuild from existing HPI/BoE files; `-skip-supply-download` skips only the Table 122 ODS. Override URLs with `UKHPI_URL`, `BOE_URL`, or `TABLE122_URL` when releases move. Optional bulk pulls: `-fetch-ppd` (Price Paid CSV; set `PPD_CSV_URL` if the portal URL changes), `-ons-csv-url`, `-earnings-csv-url`, `-nspl-zip-url` (downloads zip, extracts largest `.csv` to `dat/raw/nspl.csv`).
- **Minimal stochadex model:** [`cfg/single_la_housing.yaml`](cfg/single_la_housing.yaml) — two [`DriftDiffusionIteration`](https://github.com/umbralcalc/stochadex) processes (log earnings, log price) plus [`pkg/housing`](pkg/housing/affordability_from_logs.go) `AffordabilityFromLogsIteration` (`exp(log P − log E)` as price/earnings). This is a qualitative skeleton for forward simulation, not calibrated to data.
- **Historical spine replay:** [`cmd/runfromspine`](cmd/runfromspine/main.go) loads [`dat/processed/spine_monthly.csv`](dat/processed/spine_monthly.csv) for one pilot LA and runs [`pkg/housing`](pkg/housing/replay.go) `ReplayImplementations` — three [`FromStorageIteration`](https://github.com/umbralcalc/stochadex) partitions (log earnings, log price, affordability) built from the spine. Earnings come from `median_gross_annual_pay` when present, otherwise from `median_ratio` with price/index. Use `-validate` to check replayed affordability against ONS `median_ratio`. List LAs: `-list`.

**Still to do (see phases below):** stochastic model parameters informed by the spine (beyond deterministic replay), simulation-based inference, and the decision-science policy layer.

---

## Why This Problem

- Housing affordability has deteriorated dramatically: in 1997, 88% of local authority areas in England and Wales had median house prices below 5× median earnings. By 2021 this had fallen to just 5%. It has recovered slightly to 7% in 2025, but remains far below historical norms.
- The current government has an ambition of 1.5 million new homes over this parliament. In 2024–25, net additional dwellings were 208,600 — a 6% decrease year-on-year, and well below the roughly 300,000/year implied rate.
- Many planning permissions don't result in homes being built — the "permission to delivery" gap is one of the most contested issues in housing policy, driven by developer economics, build-out rates, and market absorption.
- Councils make planning decisions with almost no quantitative tools for understanding the housing market consequences of their choices. They approve or refuse applications one at a time, with no simulation of how a portfolio of approvals will affect local prices, affordability, and community composition over the following decade.
- The dynamics are fundamentally stochastic: house prices are noisy and correlated with interest rates, employment, migration, and sentiment; new supply takes years to materialise and its impact on prices is uncertain and contested.

---

## The Gap This Fills

| Approach | Examples | Limitation |
|----------|----------|------------|
| Hedonic price models | Standard academic house price regression | Explain price variation cross-sectionally but don't simulate forward dynamics under policy counterfactuals |
| Strategic Housing Market Assessments (SHMA) | Required for every local plan | Typically use simple demographic projections with deterministic supply assumptions; no stochastic uncertainty |
| Macro housing models | OBR, BoE housing market models | National-level, not useful for local planning decisions; treat supply as exogenous |
| Agent-based models | Academic models of buyer/seller behaviour | Theoretically rich but rarely calibrated to real transaction data; hard to validate |

**The stochadex differentiator:** a local-authority-level stochastic simulation that learns price dynamics, supply-demand interactions, and affordability trajectories from 30 years of transaction data, then evaluates planning policy portfolios with proper uncertainty quantification. Same proven pattern — ingest freely available data, build a simulation that learns from it, optimise policy actions.

---

## Phase 1: Data Ingestion

### 1.1 Property transaction data

**Source: HM Land Registry Price Paid Data**

- Every residential property sale in England and Wales lodged for registration
- Over 24 million records from January 1995 to present
- Fields: price, date, postcode, property type (detached/semi/terraced/flat), new-build flag, tenure (freehold/leasehold), full address
- Updated monthly (20th working day of each month)
- Open Government Licence, free bulk download (CSV, ~115–230MB per year)
- Also available as linked data (SPARQL endpoint) and via a report builder tool

**Download:** `gov.uk/government/statistical-data-sets/price-paid-data-downloads`

**Source: UK House Price Index (UKHPI)**

- Mix-adjusted house price index by country, region, county, and local authority
- Monthly, from January 1995
- Adjusts for the changing mix of property types sold each period — more robust than raw median prices for trend analysis
- Produced jointly by HM Land Registry, ONS, and Registers of Scotland

### 1.2 Affordability data

**Source: ONS Housing Affordability in England and Wales**

- Median and lower quartile affordability ratios (house price ÷ earnings) by local authority, annually from 1997
- Workplace-based and residence-based earnings variants
- Separate ratios for new-build and existing dwellings
- Neighbourhood-level (MSOA) affordability ratios now also available
- All downloadable as Excel/CSV from ONS

**Source: ONS Annual Survey of Hours and Earnings (ASHE)**

- Median and percentile earnings by local authority, sector, occupation
- The earnings denominator in affordability ratios
- Annual, from ONS

### 1.3 Housing supply data

**Source: MHCLG Housing Supply: Net Additional Dwellings**

- Annual net change in dwelling stock by local authority (new build + conversions + change of use − demolitions)
- The primary and most comprehensive measure of housing supply
- Live tables downloadable from gov.uk (Tables 118, 120, 122, 1000)
- From 2006–07 onwards, with tenure split (private enterprise, housing association, local authority)

**Source: MHCLG Housing Supply Indicators (Quarterly)**

- Building control reported starts and completions, seasonally adjusted
- Planning permissions granted (units), from Glenigan data
- EPC lodgements for new dwellings (a leading indicator)
- Quarterly, by local authority

**Source: MHCLG Planning Applications Statistics**

- Number of planning applications submitted and decided, by local authority
- Quarterly, with breakdowns by type (major/minor residential, etc.)

**Source: DLUHC Open Data Communities**

- Linked data platform with net additional dwellings, affordable housing supply, and other housing statistics by local authority
- Available as linked data with SPARQL endpoint

### 1.4 Demographic and economic context

**Source: ONS Population Estimates and Projections**

- Mid-year population estimates by local authority, age, and sex
- Subnational population projections (10-year and 25-year horizon)
- Internal migration estimates between local authorities

**Source: ONS/DWP Labour Market Statistics (NOMIS)**

- Claimant count, employment rate, job vacancies by travel-to-work area
- Sector composition of local employment
- The economic driver of housing demand

**Source: Bank of England**

- Bank Rate (monthly), mortgage lending data
- Interest rates are a key exogenous driver of house prices and affordability — a 1% rate change can shift affordability ratios significantly

### 1.5 Housing stock characteristics

**Source: VOA Council Tax Stock of Properties**

- Number of dwellings by council tax band, local authority
- Annual, indicating the composition and value distribution of existing stock

**Source: EPC Data (Open Data Communities)**

- Energy Performance Certificate data for all domestic properties assessed
- Includes floor area, property type, construction age, energy rating
- Linkable to other datasets via UPRN

### 1.6 Initial data scope

- **Geography:** 10–20 local authorities spanning different market types — e.g., a London borough (Tower Hamlets), a commuter belt authority (St Albans), a Northern city (Leeds), a coastal town (Brighton), a rural area (Herefordshire), and affordable areas (Burnley, Blaenau Gwent)
- **Time window:** 1995–2025 for price dynamics (30 years of Land Registry data); 2006–2025 for supply dynamics
- **Resolution:** Monthly for prices, quarterly for supply indicators, annual for affordability and demographics

---

## Phase 2: Model Structure

### 2.1 State variables

The stochadex simulation tracks a local housing market as a coupled stochastic system:

1. **House price process** — stochastic, with drift driven by fundamentals (earnings growth, interest rates, supply-demand balance) and noise driven by sentiment, credit conditions, and idiosyncratic local factors. The key observable.
2. **Housing supply pipeline** — permissions → starts → completions, with stochastic delays and attrition (not all permissions get built). This is the planning policy lever.
3. **Demand process** — population growth (natural increase + net migration) × household formation rate × mortgage availability. Stochastic, driven by employment, interest rates, and demographic trends.
4. **Affordability state** — derived: price ÷ earnings. The outcome metric that planning policy aims to influence.
5. **Stock composition** — evolving mix of property types, tenures, and sizes, shaped by what gets built and what exists.

### 2.2 Simulation diagram

```
┌─────────────────────────────────────────────────────────┐
│             MACROECONOMIC ENVIRONMENT                    │
│  Interest rates (BoE), earnings growth (ASHE),          │
│  employment (NOMIS) — exogenous stochastic drivers      │
└───┬──────────────┬─────────────────┬────────────────────┘
    │              │                 │
    ▼              ▼                 ▼
┌──────────┐ ┌───────────┐ ┌─────────────────────────────┐
│ MORTGAGE │ │  LOCAL     │ │   DEMOGRAPHIC DEMAND         │
│ AFFORDAB.│ │  EARNINGS  │ │   Population projection      │
│ (rate ×  │ │  GROWTH    │ │   + net migration             │
│  price)  │ │            │ │   + household formation       │
└────┬─────┘ └─────┬─────┘ └────────────┬────────────────┘
     │             │                     │
     └─────────────┼─────────────────────┘
                   ▼
┌─────────────────────────────────────────────────────────┐
│              DEMAND-SUPPLY BALANCE                        │
│  Net demand = demographic demand − new supply delivered  │
│  + investor demand, − second home restrictions, etc.     │
└────┬────────────────────────────────────────────────────┘
     │
     ▼
┌─────────────────────────────────────────────────────────┐
│             HOUSE PRICE DYNAMICS                          │
│  Stochastic: drift = f(demand-supply, earnings, rates)  │
│  Noise = sentiment, credit conditions, local shocks     │
│  Learned from 30 years of Land Registry transactions    │
│  By property type: detached, semi, terraced, flat       │
└────┬────────────────────────────────────────────────────┘
     │ ÷ earnings
     ▼
┌─────────────────────────────────────────────────────────┐
│             AFFORDABILITY STATE                           │
│  Price/earnings ratio by local authority                 │
│  Lower quartile ratio (for first-time buyers)           │
│  Proportion of sales below 5× threshold                 │
└─────────────────────────────────────────────────────────┘

     ▲ supply delivery (with stochastic delay + attrition)
     │
┌─────────────────────────────────────────────────────────┐
│          HOUSING SUPPLY PIPELINE (POLICY LEVER)          │
│                                                          │
│  PLANNING APPROVALS → STARTS → COMPLETIONS              │
│       (decision)     (1-2yr lag)  (2-4yr lag)           │
│                                                          │
│  By type: market housing, affordable rent, shared        │
│           ownership, social rent, build-to-rent          │
│  By density: low (houses), medium, high (flats)         │
│  By location: brownfield, greenfield, town centre        │
│  Attrition: not all permissions result in homes          │
│  Build-out rate: large sites deliver slowly              │
└─────────────────────────────────────────────────────────┘
```

### 2.3 Key modelling choices

- **Local authority level** as the primary unit, matching affordability data and planning jurisdiction. Within-LA heterogeneity captured by modelling property types separately.
- **Monthly time step** for price dynamics, quarterly for supply pipeline, to match data cadences.
- **Stochastic price model** learned from Land Registry transactions — not a theoretical asset pricing model, but an empirically-fitted stochastic process capturing the actual volatility, mean-reversion, momentum, and covariate dependence observed in the data.
- **Supply pipeline as a stochastic delay process:** permissions enter a pipeline with uncertain delay (time to start, time to complete) and uncertain attrition (probability of lapsing). These distributions are learned from the observed gap between permissions granted and completions delivered.
- **Interest rates as exogenous scenarios:** model forward rate paths as stochastic scenarios (e.g., "rates stay at 4.5%", "rates fall to 3% by 2028", "rates spike to 6%") rather than trying to predict BoE policy.
- **Ensemble approach:** run hundreds of stochastic trajectories per planning strategy to build distributions of affordability outcomes.

---

## Phase 3: Learning from Data

### 3.1 Simulation-based inference

1. **Smooth and aggregate** Land Registry transactions by local authority and property type to produce monthly price indices and transaction volumes. Combine with ASHE earnings and BoE rate data to characterise the historical covariate structure.
2. **Fit the price dynamics model** using SBI: what stochastic process (drift + diffusion + jumps) best reproduces the observed local price trajectories conditional on interest rates, earnings, and supply?
3. **Fit the supply pipeline model:** using MHCLG permissions and completions data, learn the empirical distribution of delays and attrition rates by local authority and development size.
4. **Key parameters to learn:**
   - Price elasticity of supply: how much does new supply depress (or fail to depress) local prices?
   - Permission-to-completion delay distribution (by LA, by development size)
   - Attrition rate: what fraction of permissions never get built?
   - Demand elasticity: how sensitive is local price growth to net migration, employment change, and interest rate movements?
   - Cross-type substitution: does building flats affect house prices, and vice versa?

### 3.2 The supply impact question

The politically crucial and empirically contested question: **does building more homes actually reduce prices locally?** Economic theory says yes; many residents and some empirical studies find the effect is small or zero locally because new supply attracts new demand (amenity effects, agglomeration). The stochadex can address this by:

1. Identifying natural experiments in the data — local authorities that experienced large supply shocks (e.g., major regeneration schemes) vs. comparable authorities that didn't
2. Fitting the price-supply elasticity with uncertainty, producing a posterior distribution rather than a point estimate
3. Propagating this uncertainty through the policy evaluation — "if supply elasticity is −0.05, this planning strategy reduces the affordability ratio by X; if it's −0.01, by Y"

### 3.3 Validation strategy

- **Temporal holdout:** Train on 1995–2020, predict 2021–2025 price and affordability trajectories (a demanding test given the post-COVID boom and subsequent rate rises).
- **Cross-LA validation:** Train on a subset of local authorities, predict dynamics in held-out LAs with similar characteristics.
- **Rate shock test:** Can the model reproduce the observed affordability improvement in 2023–2025 driven by the interest rate cycle?
- **Supply shock test:** Can it reproduce price dynamics in LAs that experienced large supply surges (e.g., Tower Hamlets, Barking and Dagenham)?

---

## Phase 4: Decision Science Layer

### 4.1 Policy actions to evaluate

| Policy type | How it acts in the model | Decision variables |
|-------------|--------------------------|-------------------|
| **Volume of approvals** | Increases supply pipeline flow | Total units/year approved |
| **Density mix** | Ratio of houses to flats affects price levels, build rate, and land use | % low/medium/high density |
| **Tenure mix** | Affordable housing requirements reduce market supply but directly house lower-income residents | % market, % affordable rent, % shared ownership, % social rent |
| **Location strategy** | Brownfield vs. greenfield; town centre vs. edge | Allocation across site types |
| **Build-out rate conditions** | Requiring faster delivery on large sites reduces pipeline lag | Maximum years to complete, phasing requirements |
| **Second home / investor restrictions** | Reduces demand from non-resident buyers | Council tax premium, licensing |
| **Infrastructure timing** | Schools, transport, GP capacity — enabling or constraining | Coordination with development phasing |

### 4.2 The affordability equity question

Affordability is not one number — it differs dramatically between the median buyer and the lower quartile buyer (who is likely a first-time buyer). The stochadex can evaluate policies separately for these groups, and also track the stock composition: does a strategy that maximises overall affordability do so by building luxury flats (which attract demand from outside and don't help local first-time buyers) or by building family homes at accessible price points?

### 4.3 Objective function

For each planning strategy, simulate multiple trajectories across interest rate scenarios and evaluate:

- **Primary outcome:** Expected median affordability ratio at 10 and 20 years
- **First-time buyer outcome:** Expected lower quartile affordability ratio
- **Supply delivery:** Expected cumulative net additional dwellings actually delivered (accounting for pipeline attrition)
- **Robustness metric:** Performance across interest rate scenarios (does this strategy work if rates stay high *and* if they fall?)
- **Composition metric:** Mix of property types and tenures in the resulting stock

### 4.4 Output

For a given local authority, produce actionable planning recommendations:

> *"For St Albans (current affordability ratio 13.2), approving 1,200 units/year with a 40% affordable requirement and 60% medium-density housing reduces the expected median affordability ratio to 11.4 (90% CI: 10.1 to 12.8) by 2035 under a base interest rate scenario. However, if only 70% of permissions are built out at the current attrition rate, the expected ratio is 12.1. Imposing build-out conditions that raise the completion rate to 85% improves the 2035 outcome to 10.8. Under a high-rate scenario (BoE base rate 6% sustained), the ratio falls to 9.5 regardless of supply strategy — meaning affordability improvement in that scenario is primarily rate-driven rather than supply-driven."*

---

## Phase 5: Extensions

1. **Multi-LA interactions:** Model commuter belt dynamics — prices in St Albans depend on prices in London. Build a network model of connected housing markets where supply in one LA spills demand into neighbours.
2. **Rental market:** Extend from purchase to rental affordability using VOA private rental data and ONS Index of Private Housing Rental Prices — increasingly relevant as homeownership rates fall.
3. **Spatial microsimulation:** Disaggregate to MSOA level using the ONS neighbourhood affordability data, enabling ward-level planning recommendations.
4. **Developer behaviour model:** Model housebuilders' build-out decisions as strategic (they restrict supply to maintain prices) rather than just as stochastic delays — using company accounts data from Companies House.
5. **Climate risk overlay:** Link to the flood risk project — properties in flood zones face different price dynamics and insurance costs, creating a spatial dimension to planning strategy.
6. **National policy tool:** Scale to all 318 LAs in England and Wales, producing a national planning dashboard that the government could use to evaluate the housing target of 1.5 million homes.

---

## Concrete First Steps

### Week 1–2: Data acquisition and exploration

- [ ] Download Land Registry Price Paid Data (complete file, 1995–2025) and map to LAs — use `go run ./cmd/fetchspine -fetch-ppd` (multi-GB) plus `dat/raw/nspl.csv`, or keep UK HPI–only spine without PPD columns
- [ ] ONS affordability / ASHE earnings — place CSVs in `dat/raw/` or pass `-ons-csv-url` / `-earnings-csv-url` to `fetchspine` with a **direct download URL** you obtain from ONS/NOMIS (schemas must match `ons_affordability.csv` / `earnings_annual.csv`)
- [ ] Download MHCLG Live Tables on net additional dwellings and housing starts/completions by LA (Table 122 net additions is automated; starts/completions not wired yet)
- [x] Bank of England Official Bank Rate series — fetched by `go run ./cmd/fetchspine`
- [x] UK HPI full file for monthly LA-level prices/indices — fetched by `fetchspine`, filtered to pilot LAs in `pkg/ladata/targets.yaml`
- [x] Select five pilot local authorities spanning different market types — see `pkg/ladata/targets.yaml`
- [x] Exploratory join of monthly HPI (pilot LAs) with monthly mean bank rate — output `dat/processed/spine_monthly.csv` from `fetchspine`

### Week 3–4: Minimal stochadex simulation

- [x] Implement a single-LA stochastic skeleton: log earnings and log price as `DriftDiffusionIteration`, affordability as `pkg/housing.AffordabilityFromLogsIteration` — `cfg/single_la_housing.yaml` (hand-set drift/diffusion for forward simulation)
- [x] Deterministic **replay** of the monthly spine through stochadex (`cmd/runfromspine`, `pkg/housing.ReplayImplementations` + `FromStorageIteration`)
- [ ] Add a simple supply pipeline: permissions → completions with stochastic delay
- [ ] Implement the demand-supply balance as a price pressure term
- [x] Verify the simulation runs and passes stochadex harnesses — `go test ./pkg/housing/...`; run CLI from repo root: `go run github.com/umbralcalc/stochadex/cmd/stochadex --config cfg/single_la_housing.yaml`

### Week 5–6: Simulation-based inference

- [ ] Smooth and aggregate Land Registry data into monthly price indices by LA and property type
- [ ] Set up SBI to learn price dynamics parameters conditional on macroeconomic covariates
- [ ] Fit the supply pipeline delay and attrition distributions from MHCLG data
- [ ] Validate: does the model reproduce the post-COVID boom and 2023–2025 correction?

### Week 7–8: Decision science layer

- [ ] Implement 3–4 candidate planning strategies as action sets (varying volume, density mix, tenure mix)
- [ ] Run policy evaluation across interest rate scenarios
- [ ] Produce initial findings and visualisations for the target local authorities
- [ ] Write up as a blog post in the "Engineering Smart Actions in Practice" series

---

## Key Data Sources Summary

| Source | URL | Data type | Access |
|--------|-----|-----------|--------|
| HM Land Registry Price Paid Data | gov.uk/government/statistical-data-sets/price-paid-data-downloads | Every residential sale in E&W since 1995: price, date, postcode, type, new-build flag, tenure | Free bulk download (OGL), linked data, report builder |
| UK House Price Index | landregistry.data.gov.uk | Mix-adjusted monthly price index by LA | Free download |
| ONS Housing Affordability | ons.gov.uk/peoplepopulationandcommunity/housing | Median and LQ price/earnings ratios by LA, annually from 1997 | Free download |
| ONS ASHE Earnings | ons.gov.uk | Median earnings by LA, sector, occupation | Free download |
| MHCLG Housing Supply Live Tables | gov.uk (search "live tables on housing supply") | Net additional dwellings, starts, completions, permissions by LA | Free download (OGL) |
| MHCLG Housing Supply Indicators | gov.uk (quarterly release) | Building control starts/completions, EPC lodgements, planning permissions | Free download |
| DLUHC Open Data Communities | opendatacommunities.org | Linked data: housing completions, affordable supply, tenure splits by LA | Free SPARQL endpoint |
| ONS Population Projections | ons.gov.uk | Subnational population projections by LA, age, sex | Free download |
| NOMIS Labour Market Statistics | nomisweb.co.uk | Employment, claimant count, vacancies by TTWA and LA | Free download |
| Bank of England | bankofengland.co.uk/statistics | Bank Rate, mortgage lending data | Free download |
| VOA Council Tax Stock | gov.uk | Dwellings by council tax band by LA | Free download |
| EPC Open Data | opendatacommunities.org | Energy performance certificates: floor area, age, type, rating | Free linked data |

---

## References and Related Work

- ONS Housing Affordability in England and Wales 2025 — latest release showing continued affordability improvement, with the most affordable LAs being Hyndburn and Kingston upon Hull (ratio 4.1) and least affordable Kensington and Chelsea (25.2)
- ONS "How affordable are homes in your neighbourhood?" tool (March 2026) — new MSOA-level affordability calculator comparing local house prices with local authority earnings
- MHCLG Housing Supply Indicators Q3 2025 — 208,600 net additional dwellings in 2024-25, 6% decrease YoY, well below the 1.5 million parliament target
- Land Registry Price Paid Data — 24+ million definitive records from January 1995, updated monthly, the most comprehensive transaction dataset in the UK
- DLUHC planning applications statistics — tracking the "permission to delivery" gap that is central to the policy problem