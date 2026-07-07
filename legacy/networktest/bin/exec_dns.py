#!/usr/bin/env python

from sys import path, argv
path.append('/home/net-lama/')

from modules.splib import MQTTClient
from getopt import getopt
from subprocess import Popen, PIPE
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
        "host=", 
        "server="
        ])
except:
    print("Error")

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
    elif opt in ['--server']:
        server = arg

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
def getDnsTime(host, server):
    """Do a DNS lookup and retuen the query time"""

    command = ['dig', '-4', '-u', '+timeout=1', '@' + server, host]

    p = Popen(command, stdout=PIPE, stderr=PIPE)
    output, errors = p.communicate()
    output = output.decode("utf-8").splitlines()

    for line in output:
        if 'Query time' in line:
            ms = findall('Query\stime:\s(\d+)', line)
            ms = round(float(ms[0])/1024, 2)

        if 'timed out' in line:
            ms = 3000

    data = {"Test": "DNS", "Host": host, "Server": server, "Time": ms}

    return data


dnsTime = getDnsTime(host, server)
if 'error' in dnsTime:
    client.send_log(f"DNS failed: {dnsTime}")
else:
    client.send_data(dnsTime)
    client.send_log("DNS finished, sending data to data topic")