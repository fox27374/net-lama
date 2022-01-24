#!/usr/bin/env python

import paho.mqtt.client as mqtt
from splib import checkApiEndpoint, registerClient, updateClient, getConfig, getCurrentTime
from time import sleep
from json import dumps, loads
from requests import post

clientId = False
clientType = 'HEC-Forwarder'
commands = ['start', 'stop', 'status', 'update']
dataQueue = []


capabilities = {
    'start': {
        'command': 'start',
        'description': 'Start event forwarding'
    },
    'stop': {
        'command': 'stop',
        'description': 'Stop event forwarding'
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
cmdQueue = ['start']

def mqttConnect(client, userdata, flags, rc):
    """Subscripe to MQTT topic"""
    mqttClient.subscribe([(commandTopic, 0), (dataTopic, 1)])

def mqttLog(data):
    now = getCurrentTime()
    logData = {'clientId': clientId, 'clientType': clientType, 'data': {'Time': now, 'Log': data}}
    mqttClient.publish(logTopic, dumps(logData))

def mqttMessage(client, userdata, msg):
    """Process incoming MQTT message"""
    topic = msg.topic
    message = loads((msg.payload).decode('UTF-8'))

    if topic == dataTopic:
        dataQueue.append(message)
        
    if topic == commandTopic:
        # Check if the command is for out clientId
        if message['clientId'] == clientId:
            if message['command'] in commands:
                mqttLog('Command ' + message['command'] + ' received')
                if message['command'] == 'status':
                    if cmdQueue[-1] == 'start': appStatus = 'running'
                    elif cmdQueue[-1] == 'stop': appStatus = 'stopped'
                    else: appStatus = 'undefined'
                    updateClient(clientId, clientType, appStatus, capabilities)
                    mqttLog('Sending status update to api endpoint')
                elif message['command'] == 'start':
                    cmdQueue.append('start')
                    mqttLog('Starting application')
                    updateClient(clientId, clientType, 'running', capabilities)
                    mqttLog('Sending application status update to api endpoint')
                elif message['command'] == 'stop':
                    cmdQueue.append('stop')
                    mqttLog('Stopping application')
                    updateClient(clientId, clientType, 'stopped', capabilities)
                    mqttLog('Sending application status update to api endpoint')
                elif message['command'] == 'update':
                    pass
                    # TODO
            else:
                mqttLog('Command ' + message['command'] + ' not implemented')


# Wait for the api endpoint
checkApiEndpoint()

# Register client and get ID used for further communication
# Exit if registration fails
if clientId == False:
    register = registerClient(clientType)
    if register['status'] == 'ok': clientId = register['data']['client']['clientId']
    else:
        print(f"An error occured: {register['data']}")
        sys.exit()

# Update client information at api endpoint
if cmdQueue[-1] == 'start': appStatus = 'running'
elif cmdQueue[-1] == 'stop': appStatus = 'stopped'
else: appStatus = 'undefined'
updateClient(clientId, clientType, appStatus, capabilities)

# Get config in order to connect to MQTT
mqttConfig = getConfig('configs/MQTT')
mqttServer = mqttConfig['MQTT']['mqttServer']
mqttPort = mqttConfig['MQTT']['mqttPort']
commandTopic = mqttConfig['MQTT']['commandTopic']
dataTopic = mqttConfig['MQTT']['dataTopic']
logTopic = mqttConfig['MQTT']['logTopic']

# Get application specific config
hecForwarderConfig = getConfig('configs/Hec-Forwarder')
server = hecForwarderConfig['Hec-Forwarder']['server']
port = hecForwarderConfig['Hec-Forwarder']['port']
url = hecForwarderConfig['Hec-Forwarder']['url']
token = hecForwarderConfig['Hec-Forwarder']['token']
bulk = hecForwarderConfig['Hec-Forwarder']['bulk']

# Connect to MQTT server and start subscription loop
mqttClient = mqtt.Client()
mqttClient.on_connect = mqttConnect
mqttClient.on_message = mqttMessage
mqttClient.connect(mqttServer, int(mqttPort), 60)
mqttClient.loop_start()
mqttLog(f"Client registered with clientId {clientId}")


def sendData(dataQueue):
    splunkUrl = 'http://' + server + ':' + port + url
    authHeader = {'Authorization': 'Splunk %s' %token}
    
    for data in dataQueue:
        event = {'host': 'HEC-Forwarder', 'event': data}
        print(event)
        req = post(splunkUrl, headers=authHeader, json=event, verify=False)
        mqttLog(f"Sending data to Splunk server: {req}")

# Main task, controlled by the cmdQueue switch
while True:
    if cmdQueue[-1] == 'start':
        try:
            if len(dataQueue) != 0:
                sendData(dataQueue)
                dataQueue.clear()

        except Exception as e:
            data = {'clientId': clientId, 'clientType': clientType, 'data': {'Error': e}}
            mqttLog(f"An error occured during application execution: {e}")

    else:
        pass