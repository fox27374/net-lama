#!/usr/bin/env python

from flask import Flask, render_template
from flask_wtf.csrf import CSRFProtect
from importforms.organizations import OrgAddForm, OrgDeleteForm, OrgEditForm, SiteAddForm, SiteDeleteForm, SiteEditForm
from includes.rest import RestClient
from os import urandom

debug = True
host = '10.140.80.1'
port = 5500
base_url = 'http://10.140.80.1:5000/api/v1'

SECRET_KEY = urandom(32)
csrf = CSRFProtect()

app = Flask(__name__)
app.config['SECRET_KEY'] = SECRET_KEY
app.static_folder = 'static'
csrf.init_app(app)
#app.config['WTF_CSRF_ENABLED'] = False

@app.route('/', methods = ['GET', 'POST'])
def status():
    status = rest_client.get_status()
    return render_template('status.html', status=status)

@app.route('/organizations', methods = ['GET', 'POST'])
def organizations():
    add_form = OrgAddForm()
    delete_form = OrgDeleteForm()
    edit_form = OrgEditForm()

    if add_form.validate_on_submit() and add_form.addSubmit.data:
        data = {"orgName": add_form.orgName.data}
        rest_client.add_organizations(data)

    if delete_form.validate_on_submit() and delete_form.deleteSubmit.data:
        data = delete_form.orgId.data
        rest_client.delete_organizations(data)

    if edit_form.validate_on_submit() and edit_form.editSubmit.data:
        data = {"orgId": edit_form.orgId.data, "orgName": edit_form.orgName.data}
        rest_client.put_organizations(data)

    organizations = rest_client.get_organizations()
    return render_template('organizations.html', add_form=add_form, edit_form=edit_form, delete_form=delete_form, organizations=organizations)


@app.route('/sites', methods = ['GET', 'POST'])
def sites():
    add_form = SiteAddForm()
    delete_form = SiteDeleteForm()
    edit_form = SiteEditForm()

    print(add_form.addSubmit.data)

    if add_form.validate_on_submit() and add_form.addSubmit.data:
        data = {"siteName": add_form.siteName.data, "orgId": add_form.orgId.data}
        rest_client.add_sites(data)

    if delete_form.validate_on_submit() and delete_form.deleteSubmit.data:
        data = delete_form.siteId.data
        rest_client.delete_sites(data)

    if edit_form.validate_on_submit() and edit_form.editSubmit.data:
        data = {"siteId": edit_form.siteId.data, "siteName": edit_form.siteName.data}
        rest_client.put_sites(data)

    organizations = rest_client.get_organizations()
    sitesTemp = rest_client.get_sites()
    sites = {}
    sites['Sites'] = []
    choices = []

    for site in sitesTemp['Sites']:
        orgName = rest_client.get_organizations(site['orgId'])['orgName']
        sites['Sites'].append({"siteId": site['siteId'], "siteName": site['siteName'], "orgId": site['orgId'], "orgName": orgName})

    for organization in organizations['Organizations']:
        choices.append((organization['orgId'], organization['orgName']))

    add_form.orgId.choices = choices

    return render_template('sites.html', add_form=add_form, edit_form=edit_form, delete_form=delete_form, sites=sites)

@app.route('/form', methods = ['GET', 'POST'])
def configForm():
    return render_template('configForm.html')

if __name__ == '__main__':
    rest_client = RestClient(base_url, 'Daniel', '1234')
    app.run(debug=debug, host=host, port=port)
