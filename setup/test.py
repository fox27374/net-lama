import docker
client = docker.from_env()

for network in client.networks.list():
    print(network.id)
