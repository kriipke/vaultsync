# vaultsync demo: edit a pulled secret locally (rotate a password)
Output edit.gif

Set Shell bash
Set FontSize 20
Set Width 920
Set Height 480
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms
Set Padding 20

Hide
Type "source setup.sh pulled && clear"
Enter
Sleep 500ms
Show

Type "cat secrets/app/database.yaml"
Sleep 500ms
Enter
Sleep 2s

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

Type "cat secrets/app/database.yaml"
Sleep 500ms
Enter
Sleep 4s
