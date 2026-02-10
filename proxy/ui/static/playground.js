/**
 * playground.js - Client-side functionality for playground features
 */

(function() {
  'use strict';

  const Playground = {
    sessionId: 'default',
    currentStream: null,

    // Initialize playground
    init() {
      // Use sessionStorage (tab-specific) instead of localStorage (browser-wide)
      this.sessionId = sessionStorage.getItem('playground-session-id');
      if (!this.sessionId) {
        // Generate unique session ID for this tab
        this.sessionId = 'pg-' + Date.now() + '-' + Math.random().toString(36).substr(2, 9);
        sessionStorage.setItem('playground-session-id', this.sessionId);
      }
      this.loadChatHistory();
    },

    // Load chat history on page load
    async loadChatHistory() {
      try {
        const response = await fetch(`/api/playground/chat/history?sessionId=${this.sessionId}`);
        if (!response.ok) return;

        const data = await response.json();
        if (data.messages && data.messages.length > 0) {
          const messagesContainer = document.querySelector('#chat-history .messages');
          if (messagesContainer) {
            messagesContainer.innerHTML = '';
            data.messages.forEach(msg => {
              this.appendMessage(msg.role, msg.content);
            });
          }
        }
      } catch (err) {
        console.error('Failed to load chat history:', err);
      }
    },

    // Send chat message
    async sendChatMessage() {
      const modelSelect = document.getElementById('playground-chat-model');
      const messageInput = document.getElementById('playground-chat-message');
      const systemInput = document.getElementById('playground-chat-system');
      const temperatureInput = document.getElementById('playground-chat-temperature');

      if (!modelSelect || !messageInput) return;

      const model = modelSelect.value;
      const message = messageInput.value.trim();
      const system = systemInput ? systemInput.value.trim() : '';
      const temperature = temperatureInput ? parseFloat(temperatureInput.value) : 0.7;

      if (!model || !message) {
        alert('Please select a model and enter a message');
        return;
      }

      // Append user message to UI
      this.appendMessage('user', message);

      // Clear input
      messageInput.value = '';

      // Disable send button
      const sendButton = document.getElementById('send-chat');
      if (sendButton) {
        sendButton.disabled = true;
        sendButton.textContent = 'Sending...';
      }

      try {
        // Send request
        const response = await fetch('/api/playground/chat', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            model,
            message,
            system,
            temperature,
            sessionId: this.sessionId,
          }),
        });

        if (!response.ok) {
          throw new Error(`HTTP error! status: ${response.status}`);
        }

        // Handle streaming response
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let assistantMessage = '';
        let messageDiv = null;

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          const chunk = decoder.decode(value, { stream: true });
          const lines = chunk.split('\n');

          for (const line of lines) {
            if (!line.startsWith('data: ')) continue;

            const data = line.substring(6);
            if (data === '[DONE]') continue;

            try {
              const parsed = JSON.parse(data);
              const delta = parsed.choices?.[0]?.delta?.content;

              if (delta) {
                assistantMessage += delta;

                // Create or update message div
                if (!messageDiv) {
                  messageDiv = this.appendMessage('assistant', '');
                }

                const contentDiv = messageDiv.querySelector('.content');
                if (contentDiv) {
                  contentDiv.textContent = assistantMessage;
                }
              }
            } catch (e) {
              // Ignore JSON parse errors
            }
          }
        }
      } catch (err) {
        console.error('Failed to send message:', err);
        this.appendMessage('system', `Error: ${err.message}`);
      } finally {
        // Re-enable send button
        if (sendButton) {
          sendButton.disabled = false;
          sendButton.textContent = 'Send Message';
        }
      }
    },

    // Append message to chat history
    appendMessage(role, content) {
      const messagesContainer = document.querySelector('#chat-history .messages');
      if (!messagesContainer) return null;

      const messageDiv = document.createElement('div');
      messageDiv.className = `chat-message ${role}`;

      const roleDiv = document.createElement('div');
      roleDiv.className = 'role';
      roleDiv.textContent = role;

      const contentDiv = document.createElement('div');
      contentDiv.className = 'content';
      contentDiv.textContent = content;

      messageDiv.appendChild(roleDiv);
      messageDiv.appendChild(contentDiv);

      messagesContainer.appendChild(messageDiv);

      // Scroll to bottom
      const chatHistory = document.getElementById('chat-history');
      if (chatHistory) {
        chatHistory.scrollTop = chatHistory.scrollHeight;
      }

      return messageDiv;
    },

    // Clear chat history
    async clearChatHistory() {
      if (!confirm('Clear all chat history?')) return;

      try {
        const response = await fetch(`/api/playground/chat/clear?sessionId=${this.sessionId}`, {
          method: 'POST',
        });

        if (response.ok) {
          const messagesContainer = document.querySelector('#chat-history .messages');
          if (messagesContainer) {
            messagesContainer.innerHTML = '';
          }
        }
      } catch (err) {
        console.error('Failed to clear chat:', err);
      }
    },

    // Generate image
    async generateImage() {
      const modelSelect = document.getElementById('playground-images-model');
      const promptInput = document.getElementById('playground-images-prompt');
      const sizeSelect = document.getElementById('playground-images-size');
      const generateButton = document.getElementById('generate-image');
      const resultDiv = document.getElementById('image-result');

      if (!modelSelect || !promptInput) return;

      const model = modelSelect.value;
      const prompt = promptInput.value.trim();
      const size = sizeSelect ? sizeSelect.value : '1024x1024';

      if (!model || !prompt) {
        alert('Please select a model and enter a prompt');
        return;
      }

      if (generateButton) {
        generateButton.disabled = true;
        generateButton.textContent = 'Generating...';
      }

      try {
        const response = await fetch('/api/playground/images', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ model, prompt, size }),
        });

        if (!response.ok) {
          throw new Error(`HTTP error! status: ${response.status}`);
        }

        const data = await response.json();

        if (resultDiv && data.data && data.data.length > 0) {
          const imageUrl = data.data[0].url || `data:image/png;base64,${data.data[0].b64_json}`;

          resultDiv.innerHTML = `
            <div style="margin-top: 1rem;">
              <img src="${imageUrl}" alt="Generated image" style="max-width: 100%; border-radius: 6px;" />
              <div style="margin-top: 0.5rem;">
                <a href="${imageUrl}" download="generated-image.png" class="topcoat-button">Download</a>
              </div>
            </div>
          `;
        }
      } catch (err) {
        console.error('Failed to generate image:', err);
        if (resultDiv) {
          resultDiv.innerHTML = `<p style="color: red;">Error: ${err.message}</p>`;
        }
      } finally {
        if (generateButton) {
          generateButton.disabled = false;
          generateButton.textContent = 'Generate';
        }
      }
    },

    // Generate speech
    async generateSpeech() {
      const modelSelect = document.getElementById('playground-speech-model');
      const textInput = document.getElementById('playground-speech-input');
      const voiceSelect = document.getElementById('playground-speech-voice');
      const generateButton = document.getElementById('generate-speech');
      const resultDiv = document.getElementById('speech-result');

      if (!modelSelect || !textInput) return;

      const model = modelSelect.value;
      const input = textInput.value.trim();
      const voice = voiceSelect ? voiceSelect.value : 'alloy';

      if (!model || !input) {
        alert('Please select a model and enter text');
        return;
      }

      if (generateButton) {
        generateButton.disabled = true;
        generateButton.textContent = 'Generating...';
      }

      try {
        const response = await fetch('/api/playground/speech', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ model, input, voice }),
        });

        if (!response.ok) {
          throw new Error(`HTTP error! status: ${response.status}`);
        }

        const audioBlob = await response.blob();
        const audioUrl = URL.createObjectURL(audioBlob);

        if (resultDiv) {
          resultDiv.innerHTML = `
            <div style="margin-top: 1rem;">
              <audio controls src="${audioUrl}" style="width: 100%;"></audio>
              <div style="margin-top: 0.5rem;">
                <a href="${audioUrl}" download="speech.mp3" class="topcoat-button">Download</a>
              </div>
            </div>
          `;
        }
      } catch (err) {
        console.error('Failed to generate speech:', err);
        if (resultDiv) {
          resultDiv.innerHTML = `<p style="color: red;">Error: ${err.message}</p>`;
        }
      } finally {
        if (generateButton) {
          generateButton.disabled = false;
          generateButton.textContent = 'Generate';
        }
      }
    },

    // Transcribe audio
    async transcribeAudio() {
      const modelSelect = document.getElementById('playground-audio-model');
      const fileInput = document.getElementById('playground-audio-file');
      const transcribeButton = document.getElementById('transcribe-audio');
      const resultDiv = document.getElementById('transcription-result');

      if (!modelSelect || !fileInput) return;

      const model = modelSelect.value;
      const file = fileInput.files[0];

      if (!model || !file) {
        alert('Please select a model and choose an audio file');
        return;
      }

      if (transcribeButton) {
        transcribeButton.disabled = true;
        transcribeButton.textContent = 'Transcribing...';
      }

      try {
        const formData = new FormData();
        formData.append('model', model);
        formData.append('file', file);

        const response = await fetch('/api/playground/transcribe', {
          method: 'POST',
          body: formData,
        });

        if (!response.ok) {
          throw new Error(`HTTP error! status: ${response.status}`);
        }

        const data = await response.json();

        if (resultDiv && data.text) {
          resultDiv.innerHTML = `
            <div style="margin-top: 1rem; padding: 1rem; background: #f9fafb; border-radius: 6px;">
              <p style="margin: 0; white-space: pre-wrap;">${data.text}</p>
              <div style="margin-top: 0.5rem;">
                <button class="topcoat-button" onclick="navigator.clipboard.writeText('${data.text.replace(/'/g, "\\'")}')">Copy</button>
              </div>
            </div>
          `;
        }
      } catch (err) {
        console.error('Failed to transcribe audio:', err);
        if (resultDiv) {
          resultDiv.innerHTML = `<p style="color: red;">Error: ${err.message}</p>`;
        }
      } finally {
        if (transcribeButton) {
          transcribeButton.disabled = false;
          transcribeButton.textContent = 'Transcribe';
        }
      }
    },
  };

  // Initialize on page load
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => Playground.init());
  } else {
    Playground.init();
  }

  // Expose to window
  window.sendChatMessage = () => Playground.sendChatMessage();
  window.clearChatHistory = () => Playground.clearChatHistory();
  window.generateImage = () => Playground.generateImage();
  window.generateSpeech = () => Playground.generateSpeech();
  window.transcribeAudio = () => Playground.transcribeAudio();

})();
