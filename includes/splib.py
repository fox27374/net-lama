from requests import get, post, exceptions
from datetime import datetime
from time import sleep
import globalVars as gv

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
           
def registerClient(clientType):
    """Register client at central server"""
    registered = False
    requestType = 'post'
    apiUrl = 'clients/register'
    clientData = {'client': {'clientType': clientType}}
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