#!/usr/bin/env python

import paho.mqtt.client as mqtt
import subprocess as sp
from splib import registerClient, updateClient, getConfig, getCurrentTime
from time import sleep
from json import dumps, loads
import sys
import os

clientId = False
clientType = 'WlanSensor'
commands = ['start', 'stop', 'status', 'update']
channels= [1,6,11]
scanTime = 10

capabilities = {
    'start': {
        'command': 'start',
        'description': 'Start WLAN tshark'
    },
    'stop': {
        'command': 'stop',
        'description': 'Stop WLAN tshark'
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
wlanSensorConfig = getConfig('configs/Wlan-Sensor')
iface = wlanSensorConfig['Wlan-Sensor']['interface']

# Connect to MQTT server and start subscription loop
mqttClient = mqtt.Client()
mqttClient.on_connect = mqttConnect
mqttClient.on_message = mqttMessage
mqttClient.connect(mqttServer, int(mqttPort), 60)
mqttClient.loop_start()
mqttLog('Client registered with clientId ' + clientId)

def tshark():
    cmdFilter = ['-Y', 'wlan.fc.type==0 and wlan.fc.subtype==8']
    cmd = 'tshark -i ' + iface + ' -l -e wlan.ssid -e wlan.bssid -e wlan_radio.channel -s 100 -Tek'
    mqttLog('TShark command: %s' %cmd)
    mqttLog('TShark filter: %s' %cmdFilter)
    cmd = cmd.split(' ')
    cmd += cmdFilter
    print(cmd)
    procTshark = sp.Popen(cmd, stdout=sp.PIPE)
    mqttLog('Starting tshark subprocess with PID: %s' %procTshark.pid)    

    try:
        output = procTshark.stdout.readline()
        #if output == '' and procTshark.poll() is not None:
        #    break
        if output:
            printOutput = output.strip().decode()
            if 'index' not in printOutput:
                # Filter pkt header line that is send by TShark
                pktRaw = loads(output.strip())
                pktSSID = pktRaw['layers']['wlan_ssid'][0]
                pktBSSID = pktRaw['layers']['wlan_bssid'][0]
                pktChannel = pktRaw['layers']['wlan_radio_channel'][0]

                data = {"ssid":pktSSID, "bssid":pktBSSID, "channel":pktChannel}
                mqttClient.publish(dataTopic, dumps(data))
            else:
                print(output)

        else:
            print(output)

        mqttLog('Stopping tshark subprocess with PID: %s' %procTshark.pid)
        procTshark.terminate()

    except:
        mqttLog('Stopping tshark subprocess with PID: %s' %procTshark.pid)
        procTshark.terminate()

# Main task, controlled by the cmdQueue switch
while True:
    if cmdQueue[-1] == 'start':
        try:
            tshark()
            for channel in channels:
                os.system("sudo iwconfig " + iface + " channel " + str(channel))
                mqttLog('Changing interface channel to: %s' %channel)
                sleep(int(scanTime))
        except Exception as e:
            data = {'clientId': clientId, 'clientType': clientType, 'data': {'Error': e}}
            mqttLog('An error occured during application execution: ' + e)

    else:
        pass
