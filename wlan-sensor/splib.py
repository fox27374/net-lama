from requests import get, post
from datetime import datetime
import paho.mqtt.client as mqtt
import globalVars as gv

# API calls
def registerClient(clientType):
    """Register client at central server"""
    clientDict = {'client': {'clientType': clientType}}
    response = post(url=gv.apiBaseUrl + 'clients/register', json=clientDict, headers={'Content-Type': 'application/json'})
    return response.json()

def updateClient(clientId, clientType, appStatus, capabilities):
    """Update client information and status"""
    clientDict = {'client': {'clientId': clientId, 'clientType': clientType, 'appStatus': appStatus, 'capabilities': capabilities}}
    response = post(url=gv.apiBaseUrl + 'clients/update', json=clientDict, headers={'Content-Type': 'application/json'})
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
        print(wlanInfo)
        ssid = wlanInfo['ssid']
        bssid = wlanInfo['bssid']
        channel = [wlanInfo['channel']]
        wlanValue = {'bssid':bssid, 'channel':channel}
        addValue = True
    
        # Append values if SSID exists (no duplicates)
        if ssid in wlans.keys():
            for a in wlans[ssid]:
                if a['bssid'] == bssid:
                    addValue = False
            if addValue == True:
                wlans[ssid].append(wlanValue)

        # Add new SSID
        else:
            valueList = []
            valueList.append(wlanValue)
            wlans[ssid] = valueList

    return wlans
