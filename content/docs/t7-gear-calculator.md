---
title: T7 Gear Calculator
weight: 55
---

![T7 gear calculator](/t7-gear-calculator.png)

The T7 gear calculator (Tools → T7 gear calculator) computes the `GearCal.Ratio` and
`GearCal.Range` values for the Trionic 7 manual gearbox calibration from your drivetrain:
gear ratios, final drive and tire diameter.

Pick a template (FM55, FM57 or Roffe's Quaife) or type your own values — everything
recalculates as you type. A gear ratio of `0` (no 6th gear) drops that gear from the
results.

The table shows, per gear:

- **GearCal.Ratio** — the value to enter in the T7 calibration
- **Range (±)** — the allowed deviation, computed from the tolerance percentage
- **km/h** — road speed at the representative RPM

The graph plots road speed against engine RPM for every gear. Hover over it to get a
tooltip with the exact speed in each gear at the RPM under the cursor.

The tool is a port of [t7gearcal](https://github.com/roffe/t7gearcal); the source lives in
[pkg/widgets/gearcalc](https://github.com/roffe/txlogger/tree/master/pkg/widgets/gearcalc).
