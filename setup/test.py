import docker
client = docker.from_env()

images = client.images.list(name="net-lama/*")
for image in images:
    tag = image.tags
    version = tag[0][tag[0].find(':')+1:]
    print(version)
    #client.images.get(image.id)
