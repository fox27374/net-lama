#!/usr/bin/env python

import paho.mqtt.client as mqtt
import subprocess as sp
from splib import registerClient, updateClient, getConfig, getCurrentTime, createWlanList
import globalVars as gv
from time import sleep
from json import dumps, loads
from os import system
import sys


clientId = False
clientType = 'WlanSensor'
commands = ['start', 'stop', 'status', 'update']
wlanInfos = []

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
    'scan': {
        'command': 'scan',
        'description': 'Scan for nearby WLANs'
    },
    'update': {
        'command': 'update',
        'description': 'Get configuration changes'
    }
}

# Initialise application
cmdQueue = ['scan']

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
            elif message['command'] == 'scan':
                createWlanList(message)
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
wlanSensorConfig = getConfig('configs/WlanSensor')
iface = wlanSensorConfig['WlanSensor']['interface']
channels = wlanSensorConfig['WlanSensor']['channels']
scanTime = wlanSensorConfig['WlanSensor']['scanTime']

# Get frametypes
frameTypes = getConfig('configs/Frametypes')

# Connect to MQTT server and start subscription loop
mqttClient = mqtt.Client()
mqttClient.on_connect = mqttConnect
mqttClient.on_message = mqttMessage
mqttClient.connect(mqttServer, int(mqttPort), 60)
mqttClient.loop_start()
mqttLog('Client registered with clientId ' + clientId)

def sensor():
    #cmdFilter = ['-Y', 'wlan.ta==' + scanWLANBSSIDs[0] + ' or wlan.ra==' + scanWLANBSSIDs[0] + ' or wlan.sa==' + scanWLANBSSIDs[0]]
    cmd = 'tshark -i ' + iface + ' -l -e wlan.fc.retry -e wlan.fc.type -e wlan.fc.subtype -e wlan.bssid -e wlan.ssid -e wlan.sa -e wlan.da -e wlan.ta -e wlan.ra -e wlan_radio.duration -e wlan_radio.preamble -e wlan_radio.channel -s 100 -T ek'
    cmd = cmd.split(' ')
    #cmd += cmdFilter

    procSensor = sp.Popen(cmd, stdout=sp.PIPE)
    mqttLog('Starting TShark subprocess with PID: %s' %procSensor.pid)
    while True:
        output = procSensor.stdout.readline()
        if output == '' and procSensor.poll() is not None:
            break
        if output:
            printOutput = output.strip().decode()
            # Filter pkt header line that is send by TShark
            if 'index' not in printOutput:
                pktRaw = loads(output.strip())
                print(output)
                pktTime = pktRaw['timestamp']
                pktTypeRaw = pktRaw['layers']['wlan_fc_type'][0]
                pktType = frameTypes['Frametypes'][pktTypeRaw]['Name']
                pktSubtypeRaw = pktRaw['layers']['wlan_fc_subtype'][0]
                pktSubtype = frameTypes['Frametypes'][pktTypeRaw][pktSubtypeRaw]
                pktSSID = pktBSSID = pktSA = pktDA = pktTA = pktRA = 'NA'
                pktRetry = 'False'
                if 'wlan_ssid' in pktRaw['layers'].keys(): pktSSID = pktRaw['layers']['wlan_ssid'][0]
                if 'wlan_bssid' in pktRaw['layers'].keys(): pktBSSID = pktRaw['layers']['wlan_bssid'][0]
                if 'wlan_sa' in pktRaw['layers'].keys(): pktSA = pktRaw['layers']['wlan_sa'][0]
                if 'wlan_da' in pktRaw['layers'].keys(): pktDA = pktRaw['layers']['wlan_da'][0]
                if 'wlan_ta' in pktRaw['layers'].keys(): pktTA = pktRaw['layers']['wlan_ta'][0]
                if 'wlan_ra' in pktRaw['layers'].keys(): pktRA = pktRaw['layers']['wlan_ra'][0]
                if pktRaw['layers']['wlan_fc_retry'][0] == '1': pktRetry = 'True'
                pktDuration = 0
                if 'wlan_radio_duration' in pktRaw['layers'].keys():
                    pktDuration = int(pktRaw['layers']['wlan_radio_duration'][0]) + int(pktRaw['layers']['wlan_radio_preamble'][0])
                pktChannel = pktRaw['layers']['wlan_radio_channel'][0]
                data = {"time":pktTime, "event":{"Type":pktType, "Subtype":pktSubtype, "SSID":pktSSID, "BSSID":pktBSSID, "SA":pktSA, "DA":pktDA, "TA":pktTA, "RA":pktRA, "Duration":pktDuration, "Channel":pktChannel, "Retry":pktRetry}}            
                mqttClient.publish(dataTopic, dumps(data))

def scanner():
    cmdFilter = ['-Y', 'wlan.fc.subtype==8']
    cmd = 'tshark -i ' + iface + ' -l -e wlan.bssid -e wlan.ssid -e wlan_radio.channel -e wlan_radio.signal_dbm -s 100 -T ek'
    cmd = cmd.split(' ')
    cmd += cmdFilter

    loop = 0

    procSensor = sp.Popen(cmd, stdout=sp.PIPE)
    mqttLog('Starting TShark subprocess with PID: %s' %procSensor.pid)
    while loop <= 50:
        output = procSensor.stdout.readline()
        if output == '' and procSensor.poll() is not None:
            break
        if output:
            printOutput = output.strip().decode()
            # Filter pkt header line that is send by TShark
            if 'index' not in printOutput:
                pktRaw = loads(output.strip())
                print(output)
                pktTime = pktRaw['timestamp']
                pktSSID = pktRSSI = 'NA'
                if 'wlan_ssid' in pktRaw['layers'].keys(): pktSSID = pktRaw['layers']['wlan_ssid'][0]
                if 'wlan_bssid' in pktRaw['layers'].keys(): pktBSSID = pktRaw['layers']['wlan_bssid'][0]
                if 'wlan_radio_signal_dbm' in pktRaw['layers'].keys(): pktRSSI = pktRaw['layers']['wlan_radio_signal_dbm'][0]
                pktChannel = pktRaw['layers']['wlan_radio_channel'][0]
                data = {'ssid': pktSSID, 'bssid': pktBSSID, 'rssi': pktRSSI, 'channel': pktChannel}
                #print(data)
                wlanInfos.append(data)
                loop += 1
    procSensor.terminate()
                
# Main task, controlled by the cmdQueue switch
while True:
    if cmdQueue[-1] == 'start':
        try:
            sensor()
            for channel in channels:
                system("sudo iwconfig " + iface + " channel " + str(channel))
                mqttLog('Changing interface channel to: %s' %channel)
                sleep(int(scanTime))
        except Exception as e:
            data = {'clientId': clientId, 'clientType': clientType, 'data': {'Error': e}}
            mqttLog('An error occured during application execution: ' + e)

    if cmdQueue[-1] == 'scan':
        try:
            for channel in channels:
                system("sudo iwconfig " + iface + " channel " + str(channel))
                mqttLog('Changing interface channel to: %s' %channel)
                scanner()
            b = createWlanList(wlanInfos)
            break
            
        except Exception as e:
            data = {'clientId': clientId, 'clientType': clientType, 'data': {'Error': e}}
            mqttLog('An error occured during application execution: ' + e)