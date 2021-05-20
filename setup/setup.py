#!/usr/bin/python

# Imports
import subprocess as sp
import json
import docker

# Variables
configFile = 'config.json'
client = docker.from_env()

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
    
    # Check if images needs to be build
    images = client.images.list(name="net-lama/*")
    versions = []

    # If images available, check the version
    for image in images:
        tag = image.tags
        version = tag[0][tag[0].find(':')+1:]
        #if version != config['general']['version']: versions.append(False)
        if version != '0.1': versions.append(False)
        else: versions.append(True)

    # If versions are not correct, delete the images
    print(versions)
    if False in versions:
        for image in images:
            print('Deleting image ' + image.tags[0])
            client.images.remove(image.tags[0])
   
    
    # Build docker images for the applications
    for application in config['applications']:
        appName = application['name']
        appInstall = application['install']
        appInstallType = application['installType'] if application['installType'] else config['default']['installType']

        if appInstall == 'True' and appInstallType == 'docker':
            print('Building image for ' + appName)
            try:
                client.images.build(tag='net-lama/' + appName + ':' + config['general']['version'], rm=True, path='../' + appName)
            except:
                print ('A problem occured during the build process')
            
    # Run containers

    