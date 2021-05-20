import docker
client = docker.from_env()

volumes = {
    '/home/dkofler/docker/mosquitto/config': {'bind': '/mosquitto/config', 'mode': 'rw'},
    '/home/dkofler/docker/mosquitto/data': {'bind': '/mosquitto/data', 'mode': 'rw'},
    '/home/dkofler/docker/mosquitto/log': {'bind': '/mosquitto/log', 'mode': 'rw'}
}


container = client.containers.run("eclipse-mosquitto", detach=True, name='mosquitto', ports={"1883/tcp": ('10.140.80.1', 1883)}, volumes=volumes)
    
print(container.id)
