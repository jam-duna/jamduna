// Jam Codec Analyzer

// UI initialization
function populateObjectTypes() {
    const select = document.getElementById("objectType");
    select.innerHTML = "";
  
    if (typeof jamTypes !== "undefined" && Array.isArray(jamTypes)) {
      jamTypes.forEach((type) => {
        const option = document.createElement("option");
        option.value = type.value;
        option.textContent = type.label;
        select.appendChild(option);
      });
    }
}

// Initialize UI with encode mode labels
function initUI() {
    const actionType = document.getElementById("actionType");
    const inputLabel = document.getElementById("inputLabel");
    const inputDirection = document.querySelector(".input-direction");
    
    actionType.value = "encode";
    inputLabel.textContent = "JSON";
    inputDirection.textContent = "→ Codec";
    
    updateInputText();
}

// Data handling
async function updateInputText() {
    const objectType = document.getElementById("objectType").value;
    const actionType = document.getElementById("actionType").value;
    const inputField = document.getElementById("inputText");
    const outputField = document.getElementById("outputText");
    const byteSize = document.getElementById("byteSize");
  
    outputField.value = "";
    byteSize.style.display = "none";
  
    let sampleData;
    if (typeof sample_json !== "undefined" && typeof sample_codec !== "undefined") {
      sampleData = (actionType === "encode")
        ? sample_json[objectType]
        : sample_codec[objectType];
    }
  
    if (sampleData) {
      if (typeof sampleData === "string") {
        inputField.value = sampleData;
      } else {
        inputField.value = JSON.stringify(sampleData, null, 2);
      }
      
      // Auto-trigger transform if we loaded sample data
      try {
        await performTransform();
      } catch (err) {
        console.warn('Auto-transform failed:', err);
      }
    } else {
      inputField.value = "";
    }
}

// Core encode/decode transformation
async function performTransform() {
    const actionType = document.getElementById("actionType").value;
    const objectType = document.getElementById("objectType").value;
    const inputText = document.getElementById("inputText");
    const outputField = document.getElementById("outputText");
    const byteSize = document.getElementById("byteSize");
    const button = document.getElementById("actionButton");
    const copyButton = document.querySelector('button[onclick="copyOutput()"]');

    // Clear previous state
    outputField.value = "";
    byteSize.style.display = "none";
    button.disabled = true;
    copyButton.disabled = true; // Disable copy button during transform
    button.innerHTML = '<div class="spinner"></div> Processing...';

    try {
        const response = await fetch(`/${actionType}`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ objectType, inputText: inputText.value }),
        });

        const data = await response.json();
        if (!response.ok) {
            throw new Error(data.error || 'Request failed');
        }

        const rawResult = data.result || "";

        if (actionType === "encode") {
            const rawNo0x = rawResult.startsWith("0x") ? rawResult.slice(2) : rawResult;
            if (/^[0-9A-Fa-f]*$/.test(rawNo0x)) {
              const bytes = rawNo0x.length / 2;
              byteSize.textContent = `Codec size: `;
              const bytesSpan = document.createElement('strong');
              bytesSpan.textContent = bytes;
              byteSize.appendChild(bytesSpan);
              byteSize.appendChild(document.createTextNode(' bytes'));
              byteSize.style.display = "block";
            }
            outputField.value = rawResult;
          } else {
            const inputNo0x = inputText.value.startsWith("0x") ? inputText.value.slice(2) : inputText.value;
            if (/^[0-9A-Fa-f]*$/.test(inputNo0x)) {
              const bytes = inputNo0x.length / 2;
              byteSize.textContent = `Codec size: `;
              const bytesSpan = document.createElement('strong');
              bytesSpan.textContent = bytes;
              byteSize.appendChild(bytesSpan);
              byteSize.appendChild(document.createTextNode(' bytes'));
              byteSize.style.display = "block";
            }
      
            try {
              outputField.value = JSON.stringify(JSON.parse(rawResult), null, 2);
            } catch {
              outputField.value = rawResult;
            }
          }

        // On success, enable copy button
        copyButton.disabled = false;
    } catch (error) {
        console.error("Error:", error);
        outputField.value = ""; // Clear output on error
        byteSize.style.display = "none"; // Hide byte size
        copyButton.disabled = true; // Keep copy button disabled on error
        showToast(`⚠️ Error: ${error.message}`);
    } finally {
        button.disabled = false;
        button.textContent = actionType === "encode" ? "Encode" : "Decode";
    }
}

// Action handlers
window.clearFields = function() {
    document.getElementById("inputText").value = "";
    document.getElementById("outputText").value = "";
    document.getElementById("byteSize").style.display = "none";
    document.querySelector('button[onclick="copyOutput()"]').disabled = true;
};

window.swapAction = async function() {
    const actionSelect = document.getElementById("actionType");
    const inputField = document.getElementById("inputText");
    const outputField = document.getElementById("outputText");
    const inputLabel = document.getElementById("inputLabel");
    const inputDirection = document.querySelector(".input-direction");
    
    await performTransform();
    
    const oldOutput = outputField.value;
    const newMode = actionSelect.value === "encode" ? "decode" : "encode";
    
    actionSelect.value = newMode;
    inputField.value = oldOutput;
    outputField.value = "";
    
    // Update input label and direction
    if (newMode === "encode") {
        inputLabel.textContent = "JSON";
        inputDirection.textContent = "→ Codec";
    } else {
        inputLabel.textContent = "Codec";
        inputDirection.textContent = "→ JSON";
    }
    
    await performTransform();
};

window.copyOutput = async function() {
    const output = document.getElementById("outputText");
    const objectType = document.getElementById("objectType").value;
    const actionType = document.getElementById("actionType").value;
    
    // Find the human-readable label for this data structure
    const dataTypeLabel = jamTypes.find(t => t.value === objectType)?.label || objectType;
    
    if (!navigator.clipboard) {
      output.select();
      document.execCommand("copy");
      showToast(`✅ ${dataTypeLabel} copied`);
      return;
    }
    try {
      await navigator.clipboard.writeText(output.value);
      showToast(`✅ ${dataTypeLabel} copied to clipboard!`);
    } catch (err) {
      console.warn("Async clipboard failed. Using fallback.", err);
      output.select();
      document.execCommand("copy");
      showToast(`✅ ${dataTypeLabel} copied`);
    }
};

// Utilities
function showToast(message) {
    const toast = document.createElement("div");
    toast.className = "toast";
    toast.textContent = message;
    document.body.appendChild(toast);
    setTimeout(() => toast.remove(), 3000);
}

// Startup
window.addEventListener("load", () => {
    populateObjectTypes();
    initUI();
    // Only keep objectType change listener
    document.getElementById("objectType").addEventListener("change", updateInputText);
    document.getElementById("actionButton").addEventListener("click", performTransform);
});
