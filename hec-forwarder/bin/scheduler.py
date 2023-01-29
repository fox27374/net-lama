#!/usr/bin/env python

from sys import path
path.append('/home/net-lama/')

from modules.splib import MQTTClient, getClientInfo, getConfig
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
hecForwarderConfig = getConfig(f"configs/hecForwarder/{clientInfo['siteId']}")
print(hecForwarderConfig)
clientInfo['hecServer'] = hecForwarderConfig['hecServer']
clientInfo['hecPort'] = hecForwarderConfig['hecPort']
clientInfo['hecUrl'] = hecForwarderConfig['hecUrl']
clientInfo['hecToken'] = hecForwarderConfig['hecToken']

# Create client object
client = MQTTClient(**clientInfo)

# Initialise MQTT
client.create()

def execHecForwarder():
    command = [
        "python", 
        "bin/exec_hec_forwarder.py", 
        f"--clientId={clientInfo['clientId']}",
        f"--clientType={clientInfo['clientType']}",
        f"--mqttServer={clientInfo['mqttServer']}",
        f"--mqttPort={clientInfo['mqttPort']}",
        f"--dataTopic={clientInfo['dataTopic']}",
        f"--logTopic={clientInfo['logTopic']}",
        f"--hecServer={clientInfo['hecServer']}",
        f"--hecPort={clientInfo['hecPort']}",
        f"--hecUrl={clientInfo['hecUrl']}",
        f"--hecToken={clientInfo['hecToken']}"
        ]
    p = Popen(command)

try:
    execHecForwarder()

except Exception as e:
    #data = {'clientId': clientId, 'clientType': clientType, 'data': {'Error': e}}
    client.send_log(f"An error occured during application execution: {e}")
    print(f"An error occured during application execution: {e}")

else:
    pass

while True:
    sleep(10)
