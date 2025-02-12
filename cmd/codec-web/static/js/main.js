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
    
    if (actionType === "encode") {
        try {
            const inputField = document.getElementById("inputText");
            const parsed = JSON.parse(inputField.value);
            inputField.value = JSON.stringify(parsed, null, 2);
        } catch (err) {}
    }
    
    let apiEndpoint = document.getElementById("apiEndpointSelect")?.value || "http://localhost:8099";
    if (apiEndpoint === "custom") {
        apiEndpoint = document.getElementById("apiEndpointSelect").dataset.custom || "http://localhost:8099";
    }
    
    let apiUrl = apiEndpoint === "http://localhost:8099" ? `/api/${actionType}` : `${apiEndpoint}/api/${actionType}`;
    
    const inputText = document.getElementById("inputText");
    const outputField = document.getElementById("outputText");
    const byteSize = document.getElementById("byteSize");
    const button = document.getElementById("actionButton");
    const copyButton = document.querySelector('button[onclick="copyOutput()"]');

    outputField.value = "";
    byteSize.style.display = "none";
    button.disabled = true;
    copyButton.disabled = true;
    button.innerHTML = '<div class="spinner"></div> Processing...';

    try {
        const response = await fetch(apiUrl, {
            method: "POST",
            mode: "cors",
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

        copyButton.disabled = false;
    } catch (error) {
        console.error("Error:", error);
        outputField.value = "";
        byteSize.style.display = "none";
        copyButton.disabled = true;

        if (error.message.includes("Failed to fetch")) {
            showToast("⚠️ Codec Connection Dropped ");
        } else {
            showToast(`⚠️ Error: ${error.message}`);
        }
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
    document.getElementById("objectType").addEventListener("change", updateInputText);
    document.getElementById("actionButton").addEventListener("click", performTransform);

    const apiEndpointSelect = document.getElementById("apiEndpointSelect");
    if (apiEndpointSelect) {
        apiEndpointSelect.value = "http://localhost:8099";
        apiEndpointSelect.addEventListener("change", function() {
            if (this.value === "custom") {
                let endpoint = window.prompt("Set Custom Endpoint");
                if (endpoint?.trim()) {
                    endpoint = endpoint.trim().replace(/\/+$/, '');
                    if (endpoint === "http://localhost:8099") {
                        this.value = "http://localhost:8099";
                        this.dataset.custom = "";
                        showToast("Using Default Endpoint.");
                    } else {
                        this.dataset.custom = endpoint;
                        const displayText = endpoint.length > 20 ? endpoint.substring(0, 20) + "..." : endpoint;
                        this.options[this.selectedIndex].textContent = "Endpoint (" + displayText + ")";
                        showToast(`Endpoint Set to ${endpoint}`);
                    }
                } else {
                    this.value = "http://localhost:8099";
                    this.dataset.custom = "";
                    showToast("Using Default Endpoint.");
                }
            } else {
                this.dataset.custom = "";
                const customOption = Array.from(this.options).find(option => option.value === "custom");
                if (customOption) {
                    customOption.textContent = "Custom...";
                }
            }
        });
    }
});
