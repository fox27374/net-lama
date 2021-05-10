#!/usr/bin/env python

import paho.mqtt.client as mqtt
from splib import registerClient, updateClient, getConfig, getCurrentTime
from time import sleep
from json import dumps, loads
import sys
import speedtest

clientId = False
clientType = 'SpeedTest'
commands = ['start', 'stop', 'status', 'update']
capabilities = {
    'start': {
        'command': 'start',
        'description': 'Start speed test meassurement'
    },
    'stop': {
        'command': 'stop',
        'description': 'Stop speed test meassurement'
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
cmdQueue = ['stop']

def mqttConnect(client, userdata, flags, rc):
    """Subscripe to MQTT topic"""
    mqttClient.subscribe([(commandTopic, 0)])

def mqttLog(data):
    now = getCurrentTime()
    mqttClient.publish(logTopic, now + ' ' + clientId + ' ' + data)

def mqttMessage(client, userdata, msg):
    """Process incoming MQTT message"""
    #topic = msg.topic
    message = loads((msg.payload).decode('UTF-8'))

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

# Register client and get ID used for further communication
if clientId == False:
    register = registerClient(clientType)
    clientId = register['client']['clientId']

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
speedTestConfig = getConfig('configs/SpeedTest')
intervalSeconds = mqttConfig['SpeedTest']['intervalSeconds']

# Connect to MQTT server and start subscription loop
mqttClient = mqtt.Client()
mqttClient.on_connect = mqttConnect
mqttClient.on_message = mqttMessage
mqttClient.connect(mqttServer, int(mqttPort), 60)
mqttClient.loop_start()
mqttLog('Client registered with clientId ' + clientId)

# Main task, controlled by the cmdQueue switch
while True:
    if cmdQueue[-1] == 'start':
        try:
            servers = []
            threads = None
            s = speedtest.Speedtest()
            #s.get_servers(servers)
            #s.get_best_server()
            s.download(threads=threads)
            s.upload(threads=threads)
            results = s.results.dict()
            
            # Extract fields and convert to Mbit/s, round ms
            timestamp = results['timestamp']
            downMbit = round((results['download'])/1024/1024)
            upMbit = round((results['upload'])/1024/1024)
            pingMs = round(results['ping'])
            ip = results['client']['ip']
            isp = results['client']['isp']

            data = {'clientId': clientId, 'clientType': clientType, 'data': {'Time': timestamp, 'Down': downMbit, 'Up': upMbit, 'Ping': pingMs, 'IP': ip, 'isp': isp}}
            mqttLog('Speedtest finished, sending data to data topic')

        except Exception as e:
            data = {'clientId': clientId, 'clientType': clientType, 'data': {'Error': e}}
            mqttLog('An error occured during application execution: ' + e)

        mqttClient.publish(dataTopic, dumps(data))
        sleep(int(intervalSeconds))

    else:
        pass
