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
def createWlanList(wlan):
    frequencies = getConfig(apiBaseUrl + 'config/Frequencies')
    ssid = wlan['ssid']
    bssid = wlan['bssid']
    #channel = frequencies['fre2cha'][wlan['channel']]
    channel = [wlan['channel']]
    #rssi = wlan['rssi']
    wlanValue = {'bssid':bssid, 'channel':channel}
    #wlanValue = {'bssid':bssid, 'channel':channel, 'rssi':rssi}
    addValue = True
    
    # Append values if SSID exists (no duplicates)
    if ssid in gv.wlans.keys():
        for a in gv.wlans[ssid]:
            if a['bssid'] == bssid:
                addValue = False
        if addValue == True:
            gv.wlans[ssid].append(wlanValue)

    # Add new SSID
    else:
        valueList = []
        valueList.append(wlanValue)
        gv.wlans[ssid] = valueList
