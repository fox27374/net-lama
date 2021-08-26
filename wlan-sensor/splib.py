from requests import get, post, exceptions
from datetime import datetime
import paho.mqtt.client as mqtt
import globalVars as gv

# API calls
def processRequest(apiUrl, clientData):
    """Process requests and error handling"""
    headers = {'Content-Type': 'application/json'}
    data = ''
    status = 'success'
    try:
        if requestType == 'get':
            data = post(url=gv.apiBaseUrl + apiUrl, json=clientData, headers=headers)
        else:
            data = get(gv.apiBaseUrl + apiUrl)

    except requests.exceptions.HTTPError as errh:
        status = 'error'
        data = 'Http Error: ' + errh
    except requests.exceptions.ConnectionError as errc:
        status = 'error'
        data = 'Connection Error: ' + errc
    except requests.exceptions.Timeout as errt:
        status = 'error'
        data = 'Timeout Error: ' + errt
    except requests.exceptions.RequestException as err:
        status = 'error'
        data = 'General Error: ' + err

    if status == 'success': data = data.json()
    return {'status': status, 'data': data}


def registerClient(clientType):
    """Register client at central server"""
    apiUrl = 'clients/register'
    clientData = {'client': {'clientType': clientType}}
    response = processRequest(apiUrl, clientData)

    if response['status'] == 'error': print(response['data'])
    else:
        return response['data']

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
def getCurrentTime():
    now = datetime.now()
    currentTime = now.strftime('%Y-%m-%d %H:%M:%S')
    return currentTime

# Application specific functions
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
