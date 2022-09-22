#!/usr/bin/env python

from sys import path, argv
path.append('../includes/')

from subprocess import Popen, PIPE
from splib import MQTTClient
from getopt import getopt
from re import findall

argv = argv[1:]

try:
    opts, args = getopt(argv, "", [
        "clientId=",
        "clientType=",
        "mqttServer=", 
        "mqttPort=", 
        "dataTopic=", 
        "logTopic=",
        "host="
        ])
except Exception as e:
    print("Error: " + e)

for opt, arg in opts:
    if opt in ['--clientId']:
        clientId = arg
    elif opt in ['--clientType']:
        clientType = arg
    elif opt in ['--mqttServer']:
        mqttServer = arg
    elif opt in ['--mqttPort']:
        mqttPort = arg
    elif opt in ['--dataTopic']:
        dataTopic = arg
    elif opt in ['--logTopic']:
        logTopic = arg
    elif opt in ['--host']:
        host = arg

clientInfo = {}
clientInfo['clientId'] = clientId
clientInfo['clientType'] = clientType
clientInfo['mqttServer'] = mqttServer
clientInfo['mqttPort'] = mqttPort
clientInfo['dataTopic'] = dataTopic
clientInfo['logTopic'] = logTopic

# Create MQTT client object
client = MQTTClient(**clientInfo)

# Initialise MQTT
client.create()

# Application specific functions
def getPingTime(host):
    """Ping a host and return the average round-trip-time"""
    command = ['ping', '-4', '-n', '-i', '0.2', '-c', '5', host]

    p = Popen(command, stdout=PIPE, stderr=PIPE)
    output, errors = p.communicate()
    output = output.decode("utf-8").splitlines()

    avgTimeMs = 'NA'
    timeMs = []

    for line in output:
        if 'time=' in line:
            ms = findall('time=(\d+\.\d+)', line)
            
            if ms:
                timeMs.append(float(ms[0]))
            else:
                ms = 3000
                timeMs.append(float(ms))

            avgTimeMs = round((sum(timeMs)/len(timeMs)), 2)

    data = {"Test": "Ping", "Host": host, "Time": avgTimeMs}

    return data
        


pingTime = getPingTime(host)
if 'error' in pingTime:
    client.send_log(f"Ping failed: {pingTime}")
else:
    client.send_data(pingTime)
    client.send_log("Ping finished, sending data to data topic")