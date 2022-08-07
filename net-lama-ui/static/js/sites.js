var deleteSite = document.getElementById('deleteSite')
var editSite = document.getElementById('editSite')

editSite.addEventListener('show.bs.modal', function (event) {
  var button = event.relatedTarget
  var siteId = button.getAttribute('data-bs-siteId')
  var formSiteId = editSite.querySelector('.modal-body input')
  formSiteId.value = siteId
})

deleteSite.addEventListener('show.bs.modal', function (event) {
  var button = event.relatedTarget
  var siteId = button.getAttribute('data-bs-siteId')
  var siteName = button.getAttribute('data-bs-siteName')
  var modalMessage = deleteSite.querySelector('.modal-body h3')
  var formSiteId = deleteSite.querySelector('.modal-body input')
  formSiteId.value = siteId
  modalMessage.innerText = 'Delete the site ' + siteName + '?'
})
