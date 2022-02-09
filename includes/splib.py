from requests import get, post, exceptions
from datetime import datetime
from time import sleep
from json import load, loads, dumps
import globalVars as gv
import paho.mqtt.client as mqtt
from datetime import datetime, timedelta

# MQTT
class Client:
    def __init__(self, **mqttInfo):
        self.clientId = mqttInfo['clientId']
        self.clientType = mqttInfo['clientType']
        self.commandTopic = mqttInfo['commandTopic']
        self.dataTopic = mqttInfo['dataTopic']
        self.logTopic = mqttInfo['logTopic']
        self.mqttServer = mqttInfo['mqttServer']
        self.mqttPort = mqttInfo['mqttPort']
        self.commands = mqttInfo['commands']
        self.capabilities = mqttInfo['capabilities']
        self.cmdQueue = []

    def create(self):
        self.mqttClient = mqtt.Client()
        self.mqttClient.on_connect = self.connect
        self.mqttClient.on_message = self.message
        self.mqttClient.connect(self.mqttServer, int(self.mqttPort), 60)
        self.mqttClient.loop_start()
        self.log(f"Client registered with clientId {self.clientId}")

    def connect(self, *args):
        self.mqttClient.subscribe([(self.commandTopic, 0)])
        self.log(f"Client {self.clientId} subscribed to {self.commandTopic}")

    def log(self, data):
        now = getCurrentTime()
        clientInfo = {'clientId': self.clientId, 'clientType': self.clientType}
        logInfo = {'Time': now, 'Log': data}
        self.mqttClient.publish(self.logTopic, dumps({**clientInfo, **logInfo}))

    def data(self, data):
        now = getCurrentTime()
        clientInfo = {'clientId': self.clientId, 'clientType': self.clientType}
        logInfo = {'Time': now, 'Data': data}
        self.mqttClient.publish(self.dataTopic, dumps({**clientInfo, **logInfo}))

    def message(self, client, userdata, msg):
        message = loads((msg.payload).decode('UTF-8'))

        # Check if the command is for our clientId
        if message['clientId'] == self.clientId:
            if message['command'] in self.commands:
                self.log(f"Command {message['command']} received")
                if message['command'] == 'status':
                    if self.cmdQueue[-1] == 'start': appStatus = 'running'
                    elif self.cmdQueue[-1] == 'stop': appStatus = 'stopped'
                    else: appStatus = 'undefined'
                    updateClient(self.clientId, self.clientType, appStatus, self.capabilities)
                    self.log("Sending status update to api endpoint")
                elif message['command'] == 'start':
                    self.cmdQueue.append('start')
                    self.log("Starting application")
                    updateClient(self.clientId, self.clientType, 'running', self.capabilities)
                    self.log("Sending application status update to api endpoint")
                elif message['command'] == 'stop':
                    self.cmdQueue.append('stop')
                    self.log("Stopping application")
                    updateClient(self.clientId, self.clientType, 'stopped', self.capabilities)
                    self.log("Sending application status update to api endpoint")
                elif message['command'] == 'update':
                    pass
                    # TODO
            else:
                self.log(f"Command {message['command']} not implemented")


# API calls
def processRequest(requestType, apiUrl, clientData):
    """Process requests and error handling"""
    headers = {'Content-Type': 'application/json'}
    data = ''
    status = 'ok'
    try:
        if requestType == 'post':
            data = post(url=gv.apiBaseUrl + apiUrl, json=clientData, headers=headers)
            data.raise_for_status()
        else:
            data = get(gv.apiBaseUrl + apiUrl)
            data.raise_for_status()

    except exceptions.HTTPError as errh:
        status = 'error'
        data = 'Http Error: ' + str(errh)
    except exceptions.ConnectionError as errc:
        status = 'error'
        data = 'Connection Error: ' + str(errc)
    except exceptions.Timeout as errt:
        status = 'error'
        data = 'Timeout Error: ' + str(errt)
    except exceptions.RequestException as err:
        status = 'error'
        data = 'General Error: ' + str(err)

    if status == 'ok': data = data.json()
    return {'status': status, 'data': data}

def checkApiEndpoint():
    """Check if the API endpoint is reachable"""
    reachable = False
    while not reachable:
        response = processRequest('get', 'configs/all', '')
        if response['status'] == 'ok': reachable = True
        else:
            print(f"An error occured: {response['data']}")
            sleep(1)
           
def registerClient(clientType, clientId):
    """Register client at central server"""
    registered = False
    requestType = 'post'
    apiUrl = 'clients/register'
    clientData = {'client': {'clientType': clientType, 'clientId': clientId}}
    while not registered:
        response = processRequest(requestType, apiUrl, clientData)
        if response['status'] == 'ok': registered = True
        else:
            print(f"An error occured: {response['data']}")
            sleep(1)
    return response    

def updateClient(clientId, clientType, appStatus, capabilities):
    """Update client information and status"""
    clientDict = {'client': {'clientId': clientId, 'clientType': clientType, 'appStatus': appStatus, 'capabilities': capabilities}}
    response = post(url=gv.apiBaseUrl + 'clients/update', json=clientDict, headers={'Content-Type': 'application/json'})
    return response.json()

def updateConfig(clientType, configData):
    """Update application specific config"""
    configDict = {clientType: configData}
    response = post(url=gv.apiBaseUrl + 'configs/update', json=configDict, headers={'Content-Type': 'application/json'})
    return response.json()

def getConfig(apiUrl):
    response = get(gv.apiBaseUrl + apiUrl)
    return response.json()

# Support functions
def getClientInfo():
    with open('clientInfo.json') as inFile:
        return load(inFile)

def getCurrentTime():
    now = datetime.now()
    currentTime = now.strftime('%Y-%m-%d %H:%M:%S')
    return currentTime

# Application specific functions
def readConfig(configFile):
    with open(configFile, 'r') as cf:
        configDict = load(cf)
    return configDict

def writeConfig(configFile, configData):
    cf = open(configFile, 'w')
    cf.write(dumps(configData, indent=4))
    cf.close()

def writeClientDb(dbFile, clientData):
    cf = open(dbFile, 'w')
    cf.write(dumps(clientData, indent=4))
    cf.close()

def dbHousekeeping(minOutdated):
    now = datetime.now()
    compareTime = now - timedelta(minutes=minOutdated)
    print(compareTime)
    currentClients = readConfig(dbFile)
    newClients = []
    for client in currentClients['clients']:
        print(client['lastSeen'])
        print(client['clientId'])
        lastSeen = datetime.strptime(client['lastSeen'], '%Y-%m-%d %H:%M:%S')
        if lastSeen > compareTime:
            newClients.append(client)
    currentClients['clients'] = newClients
    writeClientDb(dbFile, currentClients)

def createWlanList(wlanInfos):
    wlans = {}
    for wlanInfo in wlanInfos:
        ssid = wlanInfo['ssid']
        if ssid not in wlans.keys():
            wlans[ssid] = []

    for wlanInfo in wlanInfos:
        ssid = wlanInfo['ssid']
        bssid = wlanInfo['bssid']
        channel = wlanInfo['channel']
        rssi = wlanInfo['rssi']
        inList = False
        for item in wlans[ssid]:
            if item['bssid'] == bssid: inList = True
        
        if inList == False:
            wlans[ssid].append({'bssid': bssid, 'channel': channel, 'rssi': rssi})
    return wlans 