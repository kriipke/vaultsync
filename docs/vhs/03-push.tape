# vaultsync demo: preview changes with --dry-run, then push back to Vault
Output push.gif

Set Shell bash
Set FontSize 20
Set Width 1150
Set Height 720
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms
Set Padding 20

Hide
Type "source setup.sh edited && clear"
Enter
Sleep 500ms
Show

Type "vaultsync push team-platform app --dry-run"
Sleep 500ms
Enter
Sleep 3.5s

Type "vaultsync push team-platform app"
Sleep 500ms
Enter
Sleep 3s

Type "vaultsync push team-platform app --dry-run"
Sleep 500ms
Enter
Sleep 1.5s

Type "# no diff output, Vault is in sync"
Sleep 3s
Enter
