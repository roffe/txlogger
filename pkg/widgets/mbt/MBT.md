# T7 MBT ignition analyser

Tools -> *MBT ignition analyser*. Requires a loaded T7 binary containing
`IgnNormCal.Map`, `IgnNormCal.m_AirXSP` and `IgnNormCal.n_EngYSP`.

It answers one question: **which cells of my ignition map are retarded for a
reason, and which are retarded for no reason?** It does that by modelling
where MBT lies for every cell and subtracting your map from it.

It is a comparison tool, not a calibration source. Read the
[Limits](#limits) section before you move a single cell.

## What it actually computes

Two separate things share the window, and confusing them is the main way to
misread the output.

### MBT timing — a model surface, not your ROM

The MBT number in any cell comes from two lines ([model.go](model.go)):

```go
func (m Model) MBT(pt Point, c Criterion) float64 {
	dev, fast := m.Burn.At(pt.RPM, pt.Air, pt.Lambda)
	return vibeAngle(dev, fast, c.Fraction) - c.Target
}
```

Spark has to happen early enough that the chosen fraction of the charge has
burned by the target crank angle. That is the burn delay, minus the target.
Written out, every cell is

```
burnDelay( Scale · (rpm/RefRPM)^RPMExp · (air/RefAir)^-LoadExp · (1 + LambdaK·(λ−Refλ))
           · (Δθd, Δθb),  Fraction )  −  Target
```

where `burnDelay` inverts the Vibe function to find the angle at which
`Fraction` has burned — exact, rather than the `Δθd + Δθb/2` approximation
for the 50% point.

**Why 45% and not 50%.** Lars Eriksson's Linköping dissertation 580 (1999),
*Spark Advance Modeling and Control*, publication 7, table 4, measures how
far each candidate descriptor's optimum drifts as the model parameters vary.
For the case where Δθd and Δθb move together — which is exactly what
`Burn.Scale` does here — the drifts are:

| descriptor | drift |
|---|---|
| 45% mass fraction burned | **1.0°** |
| 50% mass fraction burned | 3.0° |
| peak pressure position | 15° |
| optimal ignition itself | 50° |

So the tool is built on the descriptor that barely cares about the one input
it cannot measure, and peak pressure position — the best-known rule of thumb,
"MBT puts the peak at 16° ATDC" — is deliberately *not* offered as a
criterion. The paper's own recommendation, 45% at 9° ATDC, is the default:
at that setting "the ignition timing will at most be 1.2 degrees off
optimum". Heat transfer is what makes the target engine-specific (it moves
the 45% optimum by 20° across the plausible Woschni range), and this model
has no heat transfer term, so the target stays a knob.

**Your ROM's ignition values never enter this.** Nor does the cylinder
pressure model. The MBT view is a smooth analytic surface over rpm, load and
λ, and the loaded binary contributes only:

| From the binary | What it affects |
|---|---|
| `IgnNormCal.m_AirXSP`, `n_EngYSP` | the axes — you get your ROM's grid, at your ROM's breakpoints |
| `LambdaCal.MaxLoadNormTab` | which cells count as open loop, so λ steps from the closed-loop to the open-loop value at the enrichment boundary — visible as a ridge in the MBT surface |
| `IgnNormCal.Map`, via **Fit to map** only | sets `Burn.Scale`, one number, the overall height of the whole surface |

So: **before you press Fit, the absolute MBT numbers mean nothing.** They are
the default 15/25 CAD reference pair scaled by untuned exponents. The *shape*
across the map is meaningful from the start; the *level* is not.

### Peak pressure — your ROM's timing, run through the pressure model

The *Peak pressure* and *Peak pressure location* views, the *Torque lost*
view and the whole *Cycle* tab do run the pressure model, and they run it at
**your map's ignition timing**, not at MBT. "Peak pressure 92 bar" means
*if you run the timing in this cell, this is the peak you get* — which is
what you want when checking whether a region is anywhere near the head
gasket's limit.

The pressure model is the closed-form one from Ingemar Andersson's 2002
Linköping thesis (no. 962), ch. 5: a polytropic compression asymptote, a
polytropic expansion asymptote anchored by an ideal Otto constant-volume heat
release, and a Vibe function interpolating between them. Charge mass comes
straight from the map's mg/c load axis plus its fuel and a residual fraction,
so no manifold pressure or volumetric efficiency has to be guessed. It is
algebra, not an ODE — a full map is a few milliseconds.

## The views

| View | Uses map timing? | Reads as |
|---|---|---|
| **Δ MBT − map (°)** | yes | Positive = your map is retarded from modelled MBT. **The one you came for.** |
| **MBT timing (° BTDC)** | no | The model surface alone. |
| **Map timing (° BTDC)** | — | `IgnNormCal.Map` verbatim, for reference. |
| **Torque lost to retard (%)** | yes | Gross IMEP given up versus running MBT in that cell. Turns "6° retarded" into "1.8% torque", which is the number that tells you whether to care. |
| **Peak pressure (bar)** | yes | Peak cylinder pressure at the map's timing. |
| **Peak pressure location (° ATDC)** | yes | Where that peak falls. Well-phased combustion peaks around 12–20° ATDC — though this model runs 2–4° late, see the Cycle tab. |
| **Assumed λ** | — | Sanity check on the closed/open-loop split. Look here first if the MBT surface has a step you don't expect. |

## Workflow

1. **Set the engine.** Pick B204/B205/B234/B235, then fix the compression
   ratio — it varies by variant (L/R/E) and the presets carry the common
   turbo values, not yours. CR moves peak pressure a lot.
2. **Set the fuel and λ.** Petrol/E85 preset, then the closed-loop and
   open-loop λ. The open-loop value (default 0.82) should match what your
   binary actually commands at high load.
3. **Set the fit threshold** (*≤ mg/c*, default 350). This is the boundary
   between "trusted, at MBT" and "under investigation". It should sit below
   where your calibration starts pulling timing for knock.
4. **Press Fit to map.** This scales the burn model so predicted MBT matches
   your map across cells at or below the threshold, above 1200 rpm and above
   100 mg/c. Golden-section least squares over `Scale` in 0.2–4.0.
5. **Read the delta above the threshold.** The status line summarises exactly
   that region: mean retard, worst cell, and what the worst cell costs.

The Fit step is what makes the rest mean anything, and it is why the summary
line deliberately ignores everything at or below the threshold — down there
the model was fitted to that very map, so agreement is circular by
construction.

## The Cycle tab

Type an rpm and mg/c, get the modelled pressure trace from IVC to EVO for
that point, with the map's timing bilinearly interpolated at that exact
point. The first line underneath gives map vs MBT timing, λ, the burn angles
used, peak pressure and its location, and gross IMEP with the shortfall from
MBT.

The second line is the model's own report card: where 45% burned, 50% burned
and peak pressure all land **at the modelled MBT**, printed against the
values the literature expects (9°, 10°, 14–16° ATDC). Only the 45% point is
constrained — it is what MBT was solved for — so the other two are free to
disagree, and how far they miss is how far the burn shape is off. On a stock
MY2007 Aero map the 50% point lands at 10.3°, which is good agreement; peak
pressure lands around 18°, i.e. 2–4° late, which is the missing heat transfer
term showing itself. Expect that offset and do not tune it out.

Also useful for reality-checking the pressure side: a well-phased turbo four
at full load should show 70–120 bar peaking 12–20° ATDC. Numbers far outside
that mean a parameter is wrong, most likely compression ratio, residual
fraction or the burn scale.

## The knobs

Grouped in the accordion; *Burn angle* is open by default because it is the
group that decides the answer.

- **Engine** — bore, stroke, con rod, CR. Geometry only; sets the volume
  function.
- **Charge & fuel** — heating value, stoichiometric A/F, closed- and
  open-loop λ, charge temperature at IVC, residual gas temperature and
  fraction. These set the trapped mass and the heat release. **Only λ moves
  MBT** (through the burn angles); the temperatures and residual fraction
  move the pressure trace and peak pressure and nothing else. Sweeping
  residual fraction from 0.03 to 0.20 — far wider than reality — changes MBT
  by exactly 0.00° and peak pressure by ~15%. `TestXrDoesNotMoveMBT` pins
  that, so if residuals are ever coupled into the burn model the test fires.
- **Cycle** — polytropic exponents kc/ke, cv, IVC and EVO angles. Thesis
  values and Saab cam timing. Leave alone unless you know better.
- **Burn angle** — the reference Δθd (0–10% MFB) and Δθb (10–85% MFB), the
  reference point they apply at, the rpm/load/λ exponents, and the overall
  scale that Fit adjusts. Also the two that define MBT: **burn fraction**
  (default 0.45) and **its angle at MBT** (default 9° ATDC). Set 0.50 / 10°
  to compare against tools that use the more common 50% criterion.

## Limits

Read these as a set. They are why the tool reports relative numbers.

**The burn angle model is a guess with the right shape.** Burn angle is not
observable in a T7 log — nothing logs it, nothing infers it. The trends are
standard SI behaviour (angle grows slowly with speed, shrinks with load,
grows as the mixture leans), but the exponents are not measured. They were
swept against the part-load region of one stock MY2007 B235R calibration and
land within 2.9° RMS of it. One binary is not a validation. This is the
dominant uncertainty in everything the widget reports.

**Fit moves one number, not the shape.** It scales the whole surface up or
down. If the *shape* of your engine's burn behaviour differs from the model's
— different cams, different chamber, water injection — Fit cannot correct
that, and the residual shows up as a false gradient across the map.

**MBT is defined by a burn-fraction criterion, not by a torque search.** The
tool does not hunt for the model's own IMEP maximum. That optimum exists and
is cheap to compute, but the model carries no heat transfer term, so it lands
10–15° advanced of reality. Relative IMEP between two nearby timings is
sound, which is what the torque-loss figure uses; the absolute optimum is
not, so it isn't offered.

Table 4's result is therefore *adopted, not reproduced* here — verifying how
far each descriptor's optimum wanders needs a trustworthy MBT to measure
against, and this model hasn't got one. What `TestDescriptorRobustness`
checks is the weaker, non-circular half: holding 45% on target across a wide
burn-angle sweep, the 50% point still drifts 1.4° and peak pressure 3.0°, the
ordering the paper reports.

**The torque-loss figure agrees with the literature to ~15%.** Dissertation
580 gives 0.03% loss per degree off MBT, very nearly quadratic — 3% at ten
degrees. At 18.7° that rule predicts 10.5%; this model says 12.1%. Close
enough to trust the ranking of cells, not close enough to quote to one
decimal.

**Below 1200 rpm and 100 mg/c the map isn't what runs.** Idle comes from
`IgnIdleCal` and the bottom airmass columns are overrun. Those cells are
still drawn but excluded from fitting and from the summary.

**A positive delta is a question, not an instruction.** The map may be
retarded for reasons this model cannot see: knock margin on the fuel you
actually run, EGT limits, torque limiters, transient behaviour, cylinder-to-
cylinder spread. Everything here is a steady-state single-cylinder average.

**Compression ratio is per-variant and the preset is probably not yours.**
It shifts peak pressure directly.

The honest summary: the shape across the map is trustworthy, the number in
any one cell is only as good as the burn angles you fitted.

## Sources

- Ingemar Andersson, *Cylinder Pressure and Ionization Current Modeling for
  Spark Ignited Engines*, Linköping thesis 962 (2002) — ch. 5 is the
  closed-form pressure model; the polytropic exponents (5.2.3), the Vibe
  shape factors (5.10)–(5.12) and the λ dependence of burn angle (fig 6.4b)
  all come from it. Work done with Mecel, who built Trionic's ion sensing.
- Lars Eriksson, *Spark Advance Modeling and Control*, Linköping
  dissertation 580 (1999) — publication 7 supplies the choice of combustion
  descriptor (table 4), the 45% at 9° ATDC recommendation, and the
  loss-per-degree-off-MBT rule.

Neither is implementable in full from a T7 log: the ion-current inversion in
thesis 962 chs. 3–6 needs the raw ion trace at 1 CAD, and the T7's DI
cassette only hands the ECU one integrated value per combustion
(`In.U_Knock`). What is used here is the parts that need nothing but the map
axes.
