#!/usr/bin/env python

from sys import path, exit
path.append('../includes/')

from modules.splib import Client, checkApiEndpoint, getClientInfo, getConfig, registerClient, updateClient, getCurrentTime
from requests import post

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
mqttConfig = getConfig('configs/MQTT')
clientInfo['mqttServer'] = mqttConfig['MQTT']['mqttServer']
clientInfo['mqttPort'] = mqttConfig['MQTT']['mqttPort']
clientInfo['commandTopic'] = mqttConfig['MQTT']['commandTopic']
clientInfo['dataTopic'] = mqttConfig['MQTT']['dataTopic']
clientInfo['logTopic'] = mqttConfig['MQTT']['logTopic']

# Get application specific config
hecForwarderConfig = getConfig('configs/Hec-Forwarder')
server = hecForwarderConfig['Hec-Forwarder']['server']
port = hecForwarderConfig['Hec-Forwarder']['port']
url = hecForwarderConfig['Hec-Forwarder']['url']
token = hecForwarderConfig['Hec-Forwarder']['token']
bulk = hecForwarderConfig['Hec-Forwarder']['bulk']

# Create client object
client = Client(**clientInfo)

# Initialise MQTT
client.create()
client.connect()

# Initialise application
client.cmdQueue = ['start']

# Application specific functions
def sendData(dataQueue):
    splunkUrl = 'http://' + server + ':' + port + url
    authHeader = {'Authorization': 'Splunk %s' %token}
    
    for data in dataQueue:
        event = {"host": "HEC-Forwarder", "event": data}
        print(event)
        req = post(splunkUrl, headers=authHeader, json=event, verify=False)
        client.log(f"Sending data to Splunk server: {req}")

dataQueue = []

# Initialise application
client.cmdQueue = ['start']

# Register client and get ID used for further communication
# Exit if registration fails
register = registerClient(client.clientType, client.clientId)
if register['status'] == 'ok': clientId = register['data']['client']['clientId']
else:
    print(f"An error occured: {register['data']}")
    exit()

# Update client information at api endpoint
if client.cmdQueue[-1] == 'start': appStatus = 'running'
elif client.cmdQueue[-1] == 'stop': appStatus = 'stopped'
else: appStatus = 'undefined'
updateClient(client.clientId, client.clientType, appStatus, client.capabilities)


# Main task, controlled by the cmdQueue switch
while True:
    if client.cmdQueue[-1] == 'start':
        try:
            if len(dataQueue) != 0:
                sendData(dataQueue)
                dataQueue.clear()

        except Exception as e:
            #data = {'clientId': clientId, 'clientType': client.clientType, 'data': {'Error': e}}
            client.log(f"An error occured during application execution: {e}")

    else:
        pass