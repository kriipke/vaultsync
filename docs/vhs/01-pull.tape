# vaultsync demo: list secrets in Vault, pull them to local YAML files
Output pull.gif

Set Shell bash
Set FontSize 20
Set Width 920
Set Height 800
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

Type "tree secrets/"
Sleep 500ms
Enter
Sleep 2s

Type "cat secrets/app/database.yaml"
Sleep 500ms
Enter
Sleep 4s
