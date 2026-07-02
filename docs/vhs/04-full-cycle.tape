# vaultsync demo: the full cycle — list remote secrets, pull them down,
# edit one locally, preview with --dry-run, push back up to Vault.
Output full-cycle.gif

Set Shell bash
Set FontSize 20
Set Width 1150
Set Height 960
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms
Set Padding 20

Hide
Type "source setup.sh && clear"
Enter
Sleep 500ms
Show

Type "vaultsync list team-platform app"
Sleep 500ms
Enter
Sleep 2.5s

Type "vaultsync pull team-platform app"
Sleep 500ms
Enter
Sleep 2.5s

Type "vim secrets/app/database.yaml"
Sleep 500ms
Enter
Sleep 1.5s

Type ":%s/s3cr3t-old-password/n3w-r0tated-p@ssw0rd/"
Sleep 1s
Enter
Sleep 1.5s

Type ":wq"
Sleep 500ms
Enter
Sleep 1s

Type "vaultsync push team-platform app --dry-run"
Sleep 500ms
Enter
Sleep 3.5s

Type "vaultsync push team-platform app"
Sleep 500ms
Enter
Sleep 4s
