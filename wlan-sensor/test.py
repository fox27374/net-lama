#!/usr/bin/env python

from splib import registerClient, updateClient, getConfig
from json import dumps, loads

data = {'atalt-intern': [{'bssid': 'c0:25:5c:ec:bb:40', 'channel': '1', 'rssi': '-80'}, {'bssid': '38:20:56:58:28:00', 'channel': '6', 'rssi': '-94'}, {'bssid': 'a0:f8:49:74:8b:20', 'channel': '11', 'rssi': '-56'}], 'atalt-iot': [{'bssid': 'c0:25:5c:ec:bb:42', 'channel': '1', 'rssi': '-80'}, {'bssid': '38:20:56:58:28:02', 'channel': '6', 'rssi': '-96'}, {'bssid': 'a0:f8:49:74:8b:22', 'channel': '11', 'rssi': '-58'}], 'Kellerjoch32': [{'bssid': 'f4:92:bf:2d:40:45', 'channel': '1', 'rssi': '-80'}], '301282_EXT': [{'bssid': 'e4:f4:c6:d8:3b:e6', 'channel': '1', 'rssi': '-80'}], 'WLAN.Tele2.net': [{'bssid': 'f8:8e:85:f9:7d:bc', 'channel': '1', 'rssi': '-94'}], 'NA': [{'bssid': 'c0:25:5c:ec:bb:42', 'channel': '1', 'rssi': '-80'}, {'bssid': 'a0:f8:49:74:8b:22', 'channel': '11', 'rssi': '-24'}], '': [{'bssid': 'f2:81:73:f2:1c:07', 'channel': '6', 'rssi': '-100'}, {'bssid': '1a:e8:29:57:40:0a', 'channel': '11', 'rssi': '-86'}, {'bssid': '0a:12:a5:83:b1:fd', 'channel': '11', 'rssi': '-78'}], 'Xegony': [{'bssid': '2c:3a:fd:d0:f3:61', 'channel': '6', 'rssi': '-76'}, {'bssid': '2c:3a:fd:8b:1e:56', 'channel': '11', 'rssi': '-84'}], 'FRITZ!Box 7530 FR': [{'bssid': '2c:91:ab:4b:92:ed', 'channel': '11', 'rssi': '-86'}]}

