set PLAYERNAME=Player
set IPADDRESS=127.0.0.1:25200

start GW2.Main_Win64_Retail.exe ^
-playerName %PLAYERNAME% ^
-console ^
#-password password goes here if the server has a password ^
-runMultipleGameInstances ^
-Client.ServerIp %IPADDRESS% ^
#-dataPath "ModData\Default"
