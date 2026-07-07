from requests import get, post

config = {
    "apiUrl": "http://10.140.80.1:5000/api/v1",
    "clientId": "xxyyyzzz",
    "clientType": "hecForwarder"
}

class RESTClient():
    def __init__(self, config):
        self.apiUrl = config['apiUrl']
        self.clientId = config['clientId']
        self.userName = ""
        self.userPass = ""
        self.accessToken = ""
        self.refreshToken = ""

    def hello(self):
        """Send clientId to get a user created - POST"""
        endpoint = '/hello'
        endpointUrl = f"{self.apiUrl}{endpoint}"
        headers = {"Content-Type": "application/json"}
        data = {"clientId": self.clientId}

        req = post(endpointUrl, headers=headers, json=data)

        if req.status_code == 201:
            self.userName = req.json()['username']
            self.userPass = req.json()['password']

            return {"message": "user created successfull"}

        return {"message": "user creation failed"}

    def login(self):
        """Login with user credentials - POST"""
        endpoint = '/login'
        endpointUrl = f"{self.apiUrl}{endpoint}"
        headers = {"Content-Type": "application/json"}
        data = {"username": self.userName, "password": self.userPass}

        req = post(endpointUrl, headers=headers, json=data)

        if req.status_code == 200:
            self.accessToken = req.json()['access_token']
            self.refreshToken = req.json()['refresh_token']

            return {"message": "login successfull"}

        return {"message": "login failed"}

    def getConfig(self, section):
        """Get client specific config - GET"""
        endpoint = f"/configs/{section}"
        endpointUrl = f"{self.apiUrl}{endpoint}"
        headers = {"Content-Type": "application/json", "Authorization": f"Bearer {self.accessToken}"}

        req = get(endpointUrl, headers=headers)

        if req.status_code == 200:
            return req.json()

        return {"message": f"failed to get {section} config"}

    def refresh(self):
        """Refresh client token - POST"""
        pass

    


client = RESTClient(config)

print(client.hello())
print(client.login())
print(client.getConfig('mqtt'))
print(client.getConfig(config['clientType']))

