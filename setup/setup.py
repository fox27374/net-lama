#!/usr/bin/python

# Imports
import subprocess as sp
import json

# Variables
configFile = 'config.json'

# Functions
def readConfig(configFile):
    with open(configFile, 'r') as cf:
        configDict = json.load(cf)
    return configDict

# Load config file
config = readConfig(configFile)

# Check docker permissions
print('Checking requirements')
requirementsMet = []
print('Checking Docker')
try:
    dockerCmd = sp.run(['docker', 'ps'], capture_output=True, text=True, check=True)
    requirementsMet.append(True)
except sp.CalledProcessError:
    print ('Docker not running or missing permissions')
    requirementsMet.append(False)

print('Checking Python')
try:
    dockerCmd = sp.run(['python3', '--version'], capture_output=True, text=True, check=True)
    requirementsMet.append(True)
except sp.CalledProcessError:
    print ('Python not installed')
    requirementsMet.append(False)

if False in requirementsMet:
    print('Not all requirements met, cannot continue')
else:
    print('All requirements met, we are good to go')
    
    # Build docker images for the applications
    for application in config['applications']:
        appName = application['name']
        appInstall = application['install']
        appInstallType = application['installType'] if application['installType'] else config['default']['installType']

        if appInstall == 'True' and appInstallType == 'docker':
            print('Building image for ' + appName)
            buildCmd = 'docker build -t ' + appName + ' ../' + appName
            buildCmd = buildCmd.split(' ')
            try:
                sp.run(buildCmd, capture_output=True, text=True, check=True)
            except sp.CalledProcessError:
                print ('A problem occured during the build process')
