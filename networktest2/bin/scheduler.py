#!/usr/bin/env python

from sys import path, exit
path.append('../includes/')

from splib import MQTTClient, getClientInfo, getConfig
from time import sleep
from subprocess import Popen
from time import time

# Read local client config
clientInfo = getClientInfo()

# Wait until the API endpoint is available
#checkApiEndpoint()

# Get MQTT config
mqttConfig = getConfig(f"configs/mqtt/{clientInfo['siteId']}")
clientInfo['mqttServer'] = mqttConfig['mqttServer']
clientInfo['mqttPort'] = mqttConfig['mqttPort']
clientInfo['dataTopic'] = mqttConfig['dataTopic']
clientInfo['logTopic'] = mqttConfig['logTopic']

# Get Application specific config
networkTestConfig = getConfig(f"configs/networkTest/{clientInfo['siteId']}")
speedTestInterval = networkTestConfig['speedTestInterval']
pingDestination = networkTestConfig['pingDestination']
dnsQuery = networkTestConfig['dnsQuery']
dnsServer = networkTestConfig['dnsServer']

# Create client object
client = MQTTClient(**clientInfo)

# Initialise MQTT
client.create()

def execSpeedtest():
    command = [
        "python", 
        "bin/exec_speedtest.py", 
        f"--clientId={clientInfo['clientId']}",
        f"--clientType={clientInfo['clientType']}",
        f"--mqttServer={mqttConfig['mqttServer']}",
        f"--mqttPort={mqttConfig['mqttPort']}",
        f"--dataTopic={mqttConfig['dataTopic']}",
        f"--logTopic={mqttConfig['logTopic']}"
        ]
    p = Popen(command)

def execPing(host):
    command = [
        "python", 
        "bin/exec_ping.py", 
        f"--clientId={clientInfo['clientId']}",
        f"--clientType={clientInfo['clientType']}",
        f"--mqttServer={mqttConfig['mqttServer']}",
        f"--mqttPort={mqttConfig['mqttPort']}",
        f"--dataTopic={mqttConfig['dataTopic']}",
        f"--logTopic={mqttConfig['logTopic']}",
        f"--host={host}"
        ]
    p = Popen(command)

def execDns(query, server):
    command = [
        "python", 
        "bin/exec_dns.py", 
        f"--clientId={clientInfo['clientId']}",
        f"--clientType={clientInfo['clientType']}",
        f"--mqttServer={mqttConfig['mqttServer']}",
        f"--mqttPort={mqttConfig['mqttPort']}",
        f"--dataTopic={mqttConfig['dataTopic']}",
        f"--logTopic={mqttConfig['logTopic']}",
        f"--host={host}",
        f"--server={server}"
        ]
    p = Popen(command)

while True:
    try:
        counter = int(time())
        if (counter % 30) == 0: 
            for host in pingDestination:
                execPing(host)
            for server in dnsServer:
                for query in dnsQuery:
                    execDns(query, server)
        if (counter % int(speedTestInterval)) == 0:
            execSpeedtest()
        sleep(1)

    except Exception as e:
        #data = {'clientId': clientId, 'clientType': clientType, 'data': {'Error': e}}
        client.send_log(f"An error occured during application execution: {e}")
        print(f"An error occured during application execution: {e}")

    else:
        pass
