#!/usr/bin/env python

from sys import path, exit
path.append('../includes/')

from splib import *
#from splib import MqttClient
from time import sleep


clientId = False
clientType = 'NetworkTest'
mqttInfo = {}
mqttInfo['clientType'] = clientType
mqttInfo['commands'] = ['start', 'stop', 'status', 'update']
mqttInfo['capabilities'] = {
    'start': {
        'command': 'start',
        'description': 'Start network test'
    },
    'stop': {
        'command': 'stop',
        'description': 'Stop network test'
    },
    'status': {
        'command': 'status',
        'description': 'Send the current status to the api endpoint'
    },
    'update': {
        'command': 'update',
        'description': 'Get configuration changes'
    }
}

# Initialise application
#cmdQueue = ['start']




# Wait for the api endpoint
checkApiEndpoint()

# Get config in order to connect to MQTT
mqttConfig = getConfig('configs/MQTT')
mqttInfo['mqttServer'] = mqttConfig['MQTT']['mqttServer']
mqttInfo['mqttPort'] = mqttConfig['MQTT']['mqttPort']
mqttInfo['commandTopic'] = mqttConfig['MQTT']['commandTopic']
mqttInfo['dataTopic'] = mqttConfig['MQTT']['dataTopic']
mqttInfo['logTopic'] = mqttConfig['MQTT']['logTopic']

clientId = getClientId()
mqttInfo['clientId'] = clientId

mqttClient = MqttClient(**mqttInfo)
mqttClient.cmdQueue = ['start']

# Register client and get ID used for further communication
# Exit if registration fails
register = registerClient(clientType, clientId)
if register['status'] == 'ok': clientId = register['data']['client']['clientId']
else:
    print(f"An error occured: {register['data']}")
    exit()

# Update client information at api endpoint
if mqttClient.cmdQueue[-1] == 'start': appStatus = 'running'
elif mqttClient.cmdQueue[-1] == 'stop': appStatus = 'stopped'
else: appStatus = 'undefined'
updateClient(clientId, clientType, appStatus, mqttClient.capabilities)



# Construct client object

mqttClient.create()
mqttClient.connect()


# Get application specific config
networkTestConfig = getConfig('configs/NetworkTest')
speedTestInterval = networkTestConfig['NetworkTest']['speedTestInterval']
pingDestination = networkTestConfig['NetworkTest']['pingDestination']
dnsQuery = networkTestConfig['NetworkTest']['dnsQuery']
dnsServer = networkTestConfig['NetworkTest']['dnsServer']

# Connect to MQTT server and start subscription loop
# mqttClient = mqtt.Client()
# mqttClient.on_connect = mqttConnect
# mqttClient.on_message = mqttMessage
# mqttClient.connect(mqttServer, int(mqttPort), 60)
# mqttClient.loop_start()
# mqttLog(mqttClient, clientInfo, f"Client registered with clientId {clientId}")

# Main task, controlled by the cmdQueue switch
while True:
    if mqttClient.cmdQueue[-1] == 'start':
        try:
            mqttClient.log("Ping and DNS test finished")
            sleep(1)

        except Exception as e:
            mqttClient.log(f"An error occured during application execution: {e}")

    else:
        pass
