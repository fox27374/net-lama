#!/usr/bin/env python

from sys import path, exit
path.append('../includes/')

from splib import MQTTClient, checkApiEndpoint, getClientInfo, getConfig, registerClient, updateClient
from time import sleep
from subprocess import Popen
from time import time

# Read local client config
localConfig = getClientInfo()

clientInfo = {}
clientInfo['clientId'] = localConfig['clientId']
clientInfo['clientType'] = localConfig['clientType']
clientInfo['commands'] = localConfig['commands']
clientInfo['capabilities'] = localConfig['capabilities']

# Wait until the API endpoint is available
checkApiEndpoint()

# Get MQTT config
mqttConfig = getConfig('configs/mqtt/1')
clientInfo['mqttServer'] = mqttConfig['mqttServer']
clientInfo['mqttPort'] = mqttConfig['mqttPort']
clientInfo['dataTopic'] = mqttConfig['dataTopic']
clientInfo['logTopic'] = mqttConfig['logTopic']

# Get Application specific config
networkTestConfig = getConfig('configs/networkTest/1')
speedTestInterval = networkTestConfig['speedTestInterval']
pingDestination = networkTestConfig['pingDestination']
dnsQuery = networkTestConfig['dnsQuery']
dnsServer = networkTestConfig['dnsServer']

# Create client object
client = MQTTClient(**clientInfo)

# Initialise MQTT
client.create()

# Register client and get ID used for further communication
# Exit if registration fails
#register = registerClient(client.clientType, client.clientId)
#if register['status'] == 'ok': clientId = register['data']['client']['clientId']
#else:
#    print(f"An error occured: {register['data']}")
#    exit()

# Update client information at api endpoint
#if client.cmdQueue[-1] == 'start': appStatus = 'running'
#elif client.cmdQueue[-1] == 'stop': appStatus = 'stopped'
#else: appStatus = 'undefined'
#updateClient(client.clientId, client.clientType, appStatus, client.capabilities)

def execSpeedtest():
    command = [
        "python", 
        "exec_speedtest.py", 
        f"--clientId={localConfig['clientId']}",
        f"--clientType={localConfig['clientType']}",
        f"--mqttServer={mqttConfig['mqttServer']}",
        f"--mqttPort={mqttConfig['mqttPort']}",
        f"--dataTopic={mqttConfig['dataTopic']}",
        f"--logTopic={mqttConfig['logTopic']}"
        ]
    p = Popen(command)

def execPing(host):
    command = [
        "python", 
        "exec_ping.py", 
        f"--clientId={localConfig['clientId']}",
        f"--clientType={localConfig['clientType']}",
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
        "exec_dns.py", 
        f"--clientId={localConfig['clientId']}",
        f"--clientType={localConfig['clientType']}",
        f"--mqttServer={mqttConfig['mqttServer']}",
        f"--mqttPort={mqttConfig['mqttPort']}",
        f"--dataTopic={mqttConfig['dataTopic']}",
        f"--logTopic={mqttConfig['logTopic']}",
        f"--host={host}",
        f"--server={server}"
        ]
    p = Popen(command)

# Main task, controlled by the cmdQueue switch
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
