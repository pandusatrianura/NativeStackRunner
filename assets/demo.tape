# VHS Tape for NativeStackRunner (NSR) Terminal Demo
# Generate GIF with: vhs assets/demo.tape

Output assets/demo.gif

Set FontSize 15
Set Width 1200
Set Height 620
Set Padding 20
Set Theme "Catppuccin Mocha"
Set WindowBar Colorful

Type "# 🚀 NativeStackRunner: Sub-second dependencies without Docker"
Enter
Sleep 1s

Type "stack up -d"
Enter
Sleep 4s

Type "# 📦 Services booted in <400ms! Inspect connection strings:"
Enter
Sleep 500ms

Type "stack env"
Enter
Sleep 3s

Type "# ⚡ Run ephemeral CI/CD tests with automatic teardown:"
Enter
Sleep 500ms

Type "stack exec -- go test ./..."
Enter
Sleep 4s

Type "# 🛑 Clean teardown with zero orphaned processes:"
Enter
Sleep 500ms

Type "stack down"
Enter
Sleep 2s
