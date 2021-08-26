#!/usr/bin/python

# Imports
import subprocess as sp
import json
import docker
import sys

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

    # Create network
    nwName = config['docker']['nwName']
    nwSubnet = config['docker']['nwSubnet']
    nwGateway = config['docker']['nwGateway']

    ipam_pool = docker.types.IPAMPool(subnet = nwSubnet, gateway = nwGateway)
    ipam_config = docker.types.IPAMConfig(pool_configs = [ipam_pool])

    print('Creating network ' + nwName)
    print('Parameters: Subnet ' + nwSubnet + ', Gateway: ' + nwGateway)
    client.networks.create(nwName, driver = "bridge", ipam = ipam_config)
            
    # Run containers
    for application in config['applications']:
        appName = application['name']
        appInstall = application['install']
        appInstallType = application['installType'] if application['installType'] else config['default']['installType']

        if appInstall == 'True' and appInstallType == 'docker' and appName == 'net-lama':
            print('Starting container ' + appName)
            try:
                container = client.containers.run(name=appName, image='net-lama/' + appName + ':' + config['general']['version'], 
                detach=True, network=nwName, ports = {'5000/tcp': ('10.140.80.1', 5000)}, remove=True)
            except:
                print ('A problem occured during the starting process')
                print(sys.exc_info()[0])