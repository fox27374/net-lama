#!/usr/bin/python

# Imports
import subprocess as sp
import json
import docker
import sys
import os

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
    images = client.images.list(name = "net-lama/*")
    versions = []

    # If images available, check the version
    for image in images:
        tag = image.tags
        version = tag[0][tag[0].find(':')+1:]
        #if version != config['general']['version']: versions.append(False)
        if version != '1.1': versions.append(False)
        else: versions.append(True)

    # If versions are not correct, stop containers and delete images
    if False in versions:
        for container in client.containers.list():
            for image in images:
                if container.image == image:
                    print(f'Stopping container {container.name}')
                    container.stop()

        # Prune stopped containers
        client.containers.prune()

        # Delete images
        for image in images:
            print(f'Deleting image {image.tags[0]}')
            client.images.remove(image.tags[0])

   
    # Delete network if it exists
    nwName = config['docker']['nwName']
    for network in client.networks.list():
        if network.name == nwName:
            print(f'Deleting network {network.name}')
            network.remove()


    # Build docker images for the applications
    for application in config['applications']:
        appName = application['name']
        appInstall = application['install']
        appInstallType = application['installType'] if application['installType'] else config['default']['installType']

        if appInstall == 'True' and appInstallType == 'docker':
            print(f'Building image for {appName}')

            # Temporarily copy library file from outside the build context
            os.popen('cp ../modules/splib.py ../' + appName + '/splib.py')

            try:
                client.images.build(tag='net-lama/' + appName + ':' + config['general']['version'], rm=True, path='../' + appName)
                # Remove temp file
                #os.popen('rm ../' + appName + '/splib.py')
            except Exception as e:
                print (f'A problem occured during the build process: {e}')

        

    # Create network
    nwSubnet = config['docker']['nwSubnet']
    nwGateway = config['docker']['nwGateway']

    ipam_pool = docker.types.IPAMPool(subnet = nwSubnet, gateway = nwGateway)
    ipam_config = docker.types.IPAMConfig(pool_configs = [ipam_pool])

    print(f'Creating network {nwName} with parameters: Subnet {nwSubnet}, Gateway: {nwGateway}')
    client.networks.create(nwName, driver = "bridge", ipam = ipam_config)
            
    # Run containers
    for application in config['applications']:
        appName = application['name']
        appInstall = application['install']
        hostIp = application['hostIp']
        hostPort = application['hostPort']
        containerPort = application['containerPort']
        protocol = application['protocol']
        ports = {}

        if hostPort != '':
            ports = {containerPort + '/' + protocol: (hostIp, int(hostPort))}

        if appInstall == 'True':
            print(f'Starting container {appName}')
            try:
                container = client.containers.run(name = appName, image = 'net-lama/' + appName + ':' + config['general']['version'], 
                detach = True, network = nwName, ports = ports, remove = True)
            except Exception as e:
                print (f'A problem occured during the starting process: {e}')
                print(sys.exc_info()[0])