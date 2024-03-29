# 1.0.13

The default presets has been updated. Be sure to load it once from the settings menu to make sure ActualIn.n_Engine, Out.X_AccPedal & In.v_Vehicle are logged properly on Trionic 7

In earlier versions there existed different presets depending on your CAN adapter. This has been fixed and the presets are now the same for all adapters. The default presets has been updated to reflect this change

- This new window you are reading this in
- Added WHATSNEW.md that will be displayed once the first time a new version is started.
- Added ignition duty cycle (Idc) to Dashboard, loggable via Myrtilos.InjectorDutyCycle once EU0D v25 is released, display value is 0 - 100%
- Fixed a bugg in the symbol list where "ghost" duplicates of symbols would be added when the same symbol was added to the list multiple times
- Changed symbol name in symbol list to be a label instead of a textbox, also added a copy symbol name button on each row
- Added additional symbols to Trionic 7 main menu
- It's now possible to create your own presets selectable from the preset dropdown