var deleteOrg = document.getElementById('deleteOrg')
var editOrg = document.getElementById('editOrg')
var infoToast = document.getElementById('infoToast')

editOrg.addEventListener('show.bs.modal', function (event) {
  var button = event.relatedTarget
  var orgId = button.getAttribute('data-bs-orgId')
  var formOrgId = editOrg.querySelector('.modal-body input')
  formOrgId.value = orgId
})

deleteOrg.addEventListener('show.bs.modal', function (event) {
  var button = event.relatedTarget
  var orgId = button.getAttribute('data-bs-orgId')
  var orgName = button.getAttribute('data-bs-orgName')
  var modalMessage = deleteOrg.querySelector('.modal-body h3')
  var formOrgId = deleteOrg.querySelector('.modal-body input')
  formOrgId.value = orgId
  modalMessage.innerText = 'Delete the organization ' + orgName + '?'
})

document.getElementById("toastbtn").onclick = function() {
    var toastElList = [].slice.call(document.querySelectorAll('.toast'))
    var toastList = toastElList.map(function(toastEl) {
      return new bootstrap.Toast(toastEl)
    })
    toastList.forEach(toast => toast.show()) 
  }