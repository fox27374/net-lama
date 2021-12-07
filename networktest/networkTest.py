#!/usr/bin/env python

import paho.mqtt.client as mqtt
from splib import checkApiEndpoint, registerClient, updateClient, getConfig, getCurrentTime
from time import sleep
from json import dumps, loads
import sys
import speedtest
import re

clientId = False
clientType = 'NetworkTest'
commands = ['start', 'stop', 'status', 'update']
capabilities = {
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

    # Check if the command is for our clientId
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

def getPingTime(host):
    """Ping a host and return the average round-trip-time"""

    command = ['ping', '-4', '-n', '-i', '0.2', '-c', '5', host]

    p = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    output, errors = p.communicate()
    output = output.decode("utf-8").splitlines()

    host = 'NA'
    timeMs = []

    for line in output:
        if '---' in line:
            host = re.findall('---\s{1}(\S+)', line)
            host = host[0]
        if 'icmp' in line:
            ms = re.findall('time=(\d+\.\d+)', line)
            timeMs.append(float(ms[0]))

    avgTimeMs = round((sum(timeMs)/len(timeMs)), 2)

    return({"Host": host, "Time": avgTimeMs})

def getDnsTime(host, server):
    """Do a DNS lookup and retuen the query time"""

    command = ['dig', '-4', '-u', '+timeout=1', '@' + server, host]

    p = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    output, errors = p.communicate()
    output = output.decode("utf-8").splitlines()

    for line in output:
        if 'Query time' in line:
            ms = re.findall('Query\stime:\s(\d+)', line)
            ms = round(float(ms[0])/1024, 2)

        if 'timed out' in line:
            ms = 3000

    return({"Host": host, "Server": server, "Time": ms})

def getSpeedTest():
    servers = []
    threads = None
    s = speedtest.Speedtest()
    try:
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

        return data
        
    except Exception as e:
        data = {'clientId': clientId, 'clientType': clientType, 'data': {'Error': e}}
        mqttLog('An error occured during application execution: ' + e)

        return 'Error: ' + e


    
    
    


# Wait for the api endpoint
checkApiEndpoint()

# Register client and get ID used for further communication
# Exit if registration fails
if clientId == False:
    register = registerClient(clientType)
    if register['status'] == 'ok': clientId = register['data']['client']['clientId']
    else:
        print('An error occured: ' + register['data'])
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
networkTestConfig = getConfig('configs/NetworkTest')
intervalSeconds = networkTestConfig['NetworkTest']['intervalSeconds']

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
            #counter = int(intervalSeconds)
            counter = 5
            while counter >= 0:
                pingTime = getPingTime('www.google.co.uk')
                dnsTime = getDnsTime('www.google.co.uk', '8.8.4.4')
                mqttClient.publish(dataTopic, dumps(pingTime))
                mqttClient.publish(dataTopic, dumps(dnsTime))
                mqttLog('Ping and DNS test finished, sending data to data topic')
                counter = counter - 1
                sleep(1)
            speedTest = getSpeedTest()
            if 'error' in speedTest:
                pass
            else:
                mqttClient.publish(dataTopic, dumps(speedTest))
                mqttLog('Speedtest finished, sending data to data topic')
                counter = 5

        except Exception as e:
            data = {'clientId': clientId, 'clientType': clientType, 'data': {'Error': e}}
            mqttLog('An error occured during application execution: ' + e)

        #mqttClient.publish(dataTopic, dumps(data))
        #sleep(int(intervalSeconds))

    else:
        pass
