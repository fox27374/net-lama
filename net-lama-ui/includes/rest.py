from requests import get, post, put, delete

class RestClient():
    headers = {
            "Accept": "application/json",
            "Content-Type": "application/json"
    }
    access_token = ''
    refresh_token = ''

    def __init__(self, api_base_url, user_name, user_pass):
        self.api_base_url = api_base_url
        self.user_name = user_name
        self.user_pass = user_pass
        self.get_token()

    def get_token(self):
        endpoint_url = f"{self.api_base_url}/login"

        params = {
            "username": self.user_name,
            "password": self.user_pass
        }

        req = post(url=endpoint_url, headers=self.headers, json=params)
        self.access_token = req.json()['access_token']
        self.refresh_token = req.json()['refresh_token']
        self.headers['Authorization'] = f"Bearer {self.access_token}"

    def get_organizations(self, orgId=None):
        if orgId: endpoint_url = f"{self.api_base_url}/organizations/{orgId}"
        else: endpoint_url = f"{self.api_base_url}/organizations"
        req = get(url=endpoint_url, headers=self.headers)
        if req.status_code == 401:
            self.get_token()
            req = get(url=endpoint_url, headers=self.headers)
            return req.json()
        return req.json()

    def add_organizations(self, data):
        endpoint_url = f"{self.api_base_url}/organizations"
        req = post(url=endpoint_url, headers=self.headers, json=data)
        return req.json()

    def delete_organizations(self, data):
        endpoint_url = f"{self.api_base_url}/organizations/{data}"
        req = delete(url=endpoint_url, headers=self.headers)
        return req.json()

    def put_organizations(self, data):
        endpoint_url = f"{self.api_base_url}/organizations/{data['orgId']}"
        req = put(url=endpoint_url, headers=self.headers, json=data)
        return req.json()

    def get_sites(self):
        endpoint_url = f"{self.api_base_url}/sites"
        req = get(url=endpoint_url, headers=self.headers)
        if req.status_code == 401:
            self.get_token()
            req = get(url=endpoint_url, headers=self.headers)
            return req.json()
        return req.json()

    def add_sites(self, data):
        endpoint_url = f"{self.api_base_url}/sites"
        req = post(url=endpoint_url, headers=self.headers, json=data)
        return req.json()

    def delete_sites(self, data):
        endpoint_url = f"{self.api_base_url}/sites/{data}"
        req = delete(url=endpoint_url, headers=self.headers)
        return req.json()

    def put_sites(self, data):
        endpoint_url = f"{self.api_base_url}/sites/{data['siteId']}"
        req = put(url=endpoint_url, headers=self.headers, json=data)
        return req.json()    

    def get_clients(self):
        endpoint_url = f"{self.api_base_url}/clients"
        req = get(url=endpoint_url, headers=self.headers)
        return req.json()

    def get_configs(self):
        endpoint_url = f"{self.api_base_url}/configs"
        req = get(url=endpoint_url, headers=self.headers)
        return req.json()

    def get_status(self):
        status = {}
        status.update(self.get_organizations())
        status.update(self.get_sites())
        status.update(self.get_clients())
        return status