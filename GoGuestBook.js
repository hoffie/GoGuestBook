function GoGuestBook(baseURL) {
  function listEntries(callback) {
    fetch(`${baseURL}/api/entries`)
      .then(response => {
        if (!response.ok) {
          throw new Error(`HTTP error! status: ${response.status}`);
        }
        return response.json();
      })
      .then(entries => callback(null, entries))
      .catch(error => callback(error, null));
  }

  function listEntry(id, callback) {
    fetch(`${baseURL}/api/entries/${id}`)
      .then(response => {
        if (!response.ok) {
          throw new Error(`HTTP error! status: ${response.status}`);
        }
        return response.json();
      })
      .then(entries => callback(null, entries))
      .catch(error => callback(error, null));
  }

  function approveEntry(id, callback) {
    fetch(`${baseURL}/api/entries/${id}/approve`, {method: 'POST'})
      .then(response => {
        if (!response.ok) {
          throw new Error(`HTTP error! status: ${response.status}`);
        }
        return;
      })
      .then(entries => callback(null))
      .catch(error => callback(error));
  }

  function commentEntry(id, comment, callback) {
    fetch(`${baseURL}/api/entries/${id}/comment`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({comment: comment}),
    })
      .then(response => {
        if (!response.ok) {
          throw new Error(`HTTP error! status: ${response.status}`);
        }
        return;
      })
      .then(entries => callback(null))
      .catch(error => callback(error));
  }

  function rejectEntry(id, callback) {
    fetch(`${baseURL}/api/entries/${id}/reject`, {method: 'POST'})
      .then(response => {
        if (!response.ok) {
          throw new Error(`HTTP error! status: ${response.status}`);
        }
        return;
      })
      .then(entries => callback(null))
      .catch(error => callback(error));
  }

  function postEntry(entry, callback) {
    fetch(`${baseURL}/api/entries`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(entry),
    })
      .then(response => {
        if (!response.ok) {
          console.log(`HTTP error! status: ${response.status}`);
        }
        return response.json();
      })
      .then(response => callback(null, response))
      .catch(error => callback(error, null));
  }

  let entryContainer;
  const entryTemplate = document.querySelector('template#ggb-entry').content;
  const dateFormatter = new Intl.DateTimeFormat("de-DE", {
    dateStyle: "full",
    timeStyle: "short",
  });

  function renderEntry(entry) {
    const html = entryTemplate.cloneNode(true);
    if (entry.approved <= 0) {
      html.querySelector('article').classList.add("unapproved");
    }
    html.querySelector('.name').innerText = entry.name;
    html.querySelector('.created').innerText = dateFormatter.format(new Date(Date.parse(entry.created_at)));
    html.querySelector('main').innerText = entry.message;
    if (entry.comment) {
      html.querySelector('footer').innerText = entry.comment;
    } else {
      html.querySelector('footer').remove();
    }
    entryContainer.appendChild(html);
  };

  function handleRegular() {
    const mainContainer = document.querySelector('#ggb-container');
    const viewTemplate = document.querySelector('template#ggb-view');
    mainContainer.innerHTML = viewTemplate.innerHTML;
    entryContainer = document.querySelector('#ggb-entries');
    const form = document.querySelector('#ggb-form');
    const name = form.querySelector('#ggb-field-name');
    const email = form.querySelector('#ggb-field-email');
    const message = form.querySelector('#ggb-field-message');
    const code = form.querySelector('#ggb-field-code');
    const emoticons = document.querySelector('#ggb-emoticons');
    console.log(emoticons.querySelector('a'));
    emoticons.querySelectorAll('a').forEach((a) => {
      a.onclick = (el) => {
        message.value += el.target.innerText;
      };
    });
    form.onsubmit = (e) => {
      e.preventDefault();
      // FIXME validations
      const newEntry = {
        name: name.value,
        email: email.value,
        message: message.value,
        code: code.value,
      };

      postEntry(newEntry, (error, result) => {
        const statusContainer = document.querySelector('#ggb-status-container');
        if (!result) {
          console.error('Error posting entry:', result);
          statusContainer.innerText = 'Unhandled error';
          statusContainer.classList = "error";
        } else if ('error' in result) {
          console.error('Error posting entry:', result);
          const statusTemplate = document.querySelector(`template#ggb-msg-${result.error}-error`);
          statusContainer.innerHTML = statusTemplate.innerHTML;
          statusContainer.classList = "error";
        } else {
          message.value = "";
          console.log('Posted entry:', result);
          const statusTemplate = document.querySelector('template#ggb-msg-post-success');
          statusContainer.innerHTML = statusTemplate.innerHTML;
          statusContainer.classList = "success";
        }
      });
    };

    listEntries((error, entries) => {
      if (error) {
        console.error('Error listing entries:', error);
        entryContainer.innerHTML = 'Error loading guestbook contents';
      } else {
        console.log('Entries:', entries);
        entryContainer.innerText = '';
        if (entries) {
          entries.forEach(renderEntry);
        }
      }
    });
  }

  function handleAdmin(entryID) {
    function loadEntry() {
      listEntry(entryID, (error, entry) => {
        if (error) {
          console.error('Error listing entry:', error);
          entryContainer.innerHTML = 'Error loading guestbook entry';
          return;
        }
        console.log('Entry:', entry);
        entryContainer.innerText = '';
        renderEntry(entry);

        if (entry.approved <= 0) {
          document.querySelector('#ggb-container #ggb-button-approve').style.display = 'block';
          document.querySelector('#ggb-container #ggb-button-reject').style.display = 'none';
        } else {
          document.querySelector('#ggb-container #ggb-button-approve').style.display = 'none';
          document.querySelector('#ggb-container #ggb-button-reject').style.display = 'block';
        }
        document.querySelector('#ggb-container #ggb-field-comment').value = entry.comment;
        document.querySelector('#ggb-container form').style.display = 'block';
      });
    }
    const mainContainer = document.querySelector('#ggb-container');
    const adminTemplate = document.querySelector('template#ggb-admin');
    mainContainer.innerHTML = adminTemplate.innerHTML;
    entryContainer = document.querySelector('#ggb-entries');
    document.querySelector('#ggb-button-approve').onclick = (e) => {
      e.preventDefault();
      approveEntry(entryID, (error) => {
        if (error) {
          alert(error);
          return;
        }
        document.querySelector('#ggb-container #ggb-button-approve').style.display = 'none';
        document.querySelector('#ggb-container #ggb-button-reject').style.display = 'block';
        return loadEntry();
      });
    };

    document.querySelector('#ggb-button-reject').onclick = (e) => {
      e.preventDefault();
      rejectEntry(entryID, (error) => {
        if (error) {
          alert(error);
          return;
        }
        document.querySelector('#ggb-container #ggb-button-approve').style.display = 'block';
        document.querySelector('#ggb-container #ggb-button-reject').style.display = 'none';
        return loadEntry();
      });
    };

    document.querySelector('#ggb-button-comment').onclick = (e) => {
      e.preventDefault();
      const comment = document.querySelector('#ggb-field-comment').value;
      // TODO: add save indicator?
      commentEntry(entryID, comment, (error) => {
        if (error) {
          alert(error);
          return;
        }
        return loadEntry();
      });
    };

    loadEntry();
  }

  document.addEventListener("DOMContentLoaded", (event) => {
    const urlParams = new URLSearchParams(window.location.search);
    const entryID = urlParams.get('GgbEntryID');
    if (entryID) {
      return handleAdmin(entryID);
    }
    return handleRegular();
  });
}
