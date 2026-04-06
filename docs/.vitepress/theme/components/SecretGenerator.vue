<template>
  <div class="secret-generator">
    <div class="secret-type-selector">
      <button 
        @click="switchSecretType('jwt')" 
        :class="{ active: secretType === 'jwt' }">
        JWT Secret
      </button>
      <button 
        @click="switchSecretType('encryption')" 
        :class="{ active: secretType === 'encryption' }">
        Encryption Key
      </button>
      <button 
        @click="switchSecretType('password')" 
        :class="{ active: secretType === 'password' }">
        Strong Password
      </button>
    </div>
    
    <div class="secret-controls">
      <div class="length-control" v-if="secretType === 'jwt'">
        <label for="secretLength">Length:</label>
        <input 
          type="range" 
          id="secretLength" 
          v-model="secretLength" 
          min="16" 
          max="64" 
          step="8" />
        <span>{{ secretLength }} bytes</span>
      </div>
      
      <div class="length-control" v-if="secretType === 'password'">
        <label for="passwordLength">Length:</label>
        <input 
          type="range" 
          id="passwordLength" 
          v-model="passwordLength" 
          min="12" 
          max="32" 
          step="4" />
        <span>{{ passwordLength }} characters</span>
        
        <div class="password-options">
          <label>
            <input type="checkbox" v-model="passwordOptions.uppercase" />
            Uppercase
          </label>
          <label>
            <input type="checkbox" v-model="passwordOptions.numbers" />
            Numbers
          </label>
          <label>
            <input type="checkbox" v-model="passwordOptions.special" />
            Special Chars
          </label>
        </div>
      </div>
      
      <button @click="generateSecret" class="generate-button">
        Generate
      </button>
    </div>
    
    <div v-if="generatedSecret" class="secret-result">
      <div class="secret-display">
        <code>{{ generatedSecret }}</code>
      </div>
      <div class="copy-icon" @click="copyToClipboard" :class="{ copied }">
        <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
          <path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"></path>
        </svg>
        <span class="tooltip">{{ copied ? 'Copied!' : 'Copy' }}</span>
      </div>
    </div>
    
    <div v-if="passwordStrength && secretType === 'password'" class="password-strength">
      <div class="strength-meter">
        <div 
          :class="'strength-level-' + passwordStrength.score" 
          :style="{width: (passwordStrength.score + 1) * 20 + '%'}"
        ></div>
      </div>
      <div class="strength-text">
        Strength: <span :class="'strength-' + passwordStrength.score">
          {{ passwordStrengthText }}
        </span>
      </div>
      <div class="password-feedback" v-if="passwordStrength.feedback">
        {{ passwordStrength.feedback }}
      </div>
    </div>
    
    <div class="secret-info">
      <p v-if="secretType === 'jwt'">
        This JWT secret will be used for secure authentication in Teldrive.
        Place it in the <code>config.toml</code> file under the <code>[jwt]</code> section.
      </p>
      <p v-else-if="secretType === 'encryption'">
        This encryption key will be used to encrypt your files in Teldrive.
        Place it in the <code>config.toml</code> file under the <code>[tg.uploads]</code> section.
      </p>
      <p v-else>
        Use this password for secure access to your services. It's generated locally in your browser
        and never transmitted over the network.
      </p>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'

const secretType = ref('jwt')
const secretLength = ref(32)
const passwordLength = ref(20)
const generatedSecret = ref('')
const copied = ref(false)
const passwordStrength = ref(null)

const passwordOptions = ref({
  lowercase: true,
  uppercase: true,
  numbers: true,
  special: true
})

const passwordStrengthText = computed(() => {
  if (!passwordStrength.value) return '';
  
  const scores = ['Very Weak', 'Weak', 'Moderate', 'Strong', 'Very Strong'];
  return scores[passwordStrength.value.score] || 'Unknown';
})

// Function to switch between different secret types
function switchSecretType(type) {
  secretType.value = type
  generatedSecret.value = '' // Reset the generated secret
  copied.value = false
  passwordStrength.value = null
}

// Simple password strength estimator
function estimatePasswordStrength(password) {
  let score = 0;
  const length = password.length;
  
  // Length check
  if (length >= 12) score += 1;
  if (length >= 16) score += 1;
  
  // Character variety checks
  if (/[a-z]/.test(password)) score += 0.5;
  if (/[A-Z]/.test(password)) score += 0.75;
  if (/[0-9]/.test(password)) score += 0.75;
  if (/[^a-zA-Z0-9]/.test(password)) score += 1;
  
  // Pattern checks (penalize)
  if (/(.)\1{2,}/.test(password)) score -= 1; // Repeating characters
  if (/^[a-zA-Z]+$/.test(password) || /^[0-9]+$/.test(password)) score -= 1; // Only letters or only numbers
  
  // Clamp score between 0 and 4
  score = Math.max(0, Math.min(4, Math.floor(score)));
  
  let feedback = '';
  if (score < 2) {
    feedback = 'Try adding more variety with uppercase, numbers, and special characters.';
  } else if (score < 3) {
    feedback = 'Good start, but could be stronger with more complexity.';
  }
  
  return { score, feedback };
}

function generateSecret() {
  if (secretType.value === 'jwt') {
    // Use hex format for JWT (like openssl rand -hex output)
    const bytes = new Uint8Array(secretLength.value / 2)
    window.crypto.getRandomValues(bytes)
    generatedSecret.value = Array.from(bytes)
      .map(b => b.toString(16).padStart(2, '0'))
      .join('')
    passwordStrength.value = null
    
  } else if (secretType.value === 'encryption') {
    // For encryption keys, use only TOML-friendly special chars
    const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_.'
    let result = ''
    const length = 32 // Always 32 bytes (256 bits) for encryption
    
    const randomValues = new Uint8Array(length)
    window.crypto.getRandomValues(randomValues)
    
    for (let i = 0; i < length; i++) {
      result += chars.charAt(randomValues[i] % chars.length)
    }
    generatedSecret.value = result
    passwordStrength.value = null
    
  } else if (secretType.value === 'password') {
    // Generate strong password with configurable options
    let chars = 'abcdefghijklmnopqrstuvwxyz'
    if (passwordOptions.value.uppercase) chars += 'ABCDEFGHIJKLMNOPQRSTUVWXYZ'
    if (passwordOptions.value.numbers) chars += '0123456789'
    if (passwordOptions.value.special) chars += '!@#$%^&*()_+-=[]{}|;:,./<>?'
    
    const randomValues = new Uint8Array(passwordLength.value)
    window.crypto.getRandomValues(randomValues)
    
    let result = ''
    for (let i = 0; i < passwordLength.value; i++) {
      result += chars.charAt(randomValues[i] % chars.length)
    }
    generatedSecret.value = result
    
    // Estimate password strength
    passwordStrength.value = estimatePasswordStrength(result)
  }
  
  copied.value = false
}

function copyToClipboard() {
  if (!generatedSecret.value) return
  
  navigator.clipboard.writeText(generatedSecret.value).then(() => {
    copied.value = true
    setTimeout(() => {
      copied.value = false
    }, 2000)
  })
}
</script>

<style scoped>
.secret-generator {
  margin: 2rem 0;
  padding: 1.5rem;
  border-radius: 12px;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}

.secret-type-selector {
  display: flex;
  margin-bottom: 1.5rem;
  gap: 0.8rem;
  flex-wrap: wrap;
}

.secret-type-selector button {
  padding: 0.6rem 1.2rem;
  border-radius: 24px;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg);
  color: var(--vp-c-text-1);
  cursor: pointer;
  transition: all 0.2s ease;
  font-weight: 500;
}

.secret-type-selector button:hover {
  background-color: rgba(125, 125, 125, 0.05);
}

.secret-type-selector button.active {
  background-color: var(--vp-c-brand);
  color: white;
  border-color: var(--vp-c-brand-dark);
}

.length-control {
  display: flex;
  align-items: center;
  gap: 0.8rem;
  margin-bottom: 1.5rem;
  flex-wrap: wrap;
}

.password-options {
  display: flex;
  gap: 1.2rem;
  margin-top: 0.8rem;
  flex-wrap: wrap;
}

.password-options label {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  cursor: pointer;
  padding: 0.4rem 0.2rem;
  border-radius: 4px;
  transition: background-color 0.15s ease;
}

.password-options label:hover {
  background-color: rgba(125, 125, 125, 0.05);
}

.password-options input[type="checkbox"] {
  accent-color: var(--vp-c-brand);
  width: 16px;
  height: 16px;
}

.length-control input[type="range"] {
  flex-grow: 1;
  height: 6px;
  -webkit-appearance: none;
  appearance: none;
  background: linear-gradient(to right, var(--vp-c-brand-light) 0%, var(--vp-c-brand-dark) 100%);
  border-radius: 8px;
}

.length-control input[type="range"]::-webkit-slider-thumb {
  -webkit-appearance: none;
  appearance: none;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: var(--vp-c-brand);
  cursor: pointer;
  transition: transform 0.15s ease;
}

.length-control input[type="range"]::-webkit-slider-thumb:hover {
  transform: scale(1.15);
}

.generate-button {
  padding: 0.6rem 1.2rem;
  background-color: var(--vp-c-brand);
  color: white;
  border: none;
  border-radius: 24px;
  cursor: pointer;
  font-weight: 500;
  transition: background-color 0.2s ease;
}

.generate-button:hover {
  background-color: var(--vp-c-brand-dark);
}

.secret-result {
  margin-top: 1.5rem;
  border-radius: 8px;
  position: relative;
  border: 1px solid rgba(125, 125, 125, 0.1);
}

.secret-display {
  padding: 1.2rem;
  background-color: rgba(125, 125, 125, 0.04);
  border-radius: 8px;
  font-family: var(--vp-font-family-mono);
  font-size: 0.95rem;
  overflow-x: auto;
  white-space: nowrap;
  padding-right: 2.5rem; /* Make room for the copy icon */
}

.copy-icon {
  position: absolute;
  top: 1rem;
  right: 1rem;
  color: var(--vp-c-text-2);
  cursor: pointer;
  padding: 0.5rem;
  border-radius: 4px;
  transition: all 0.2s ease;
}

.copy-icon:hover {
  color: var(--vp-c-brand);
  background-color: rgba(125, 125, 125, 0.1);
}

.copy-icon.copied {
  color: var(--vp-c-brand);
}

.copy-icon .tooltip {
  position: absolute;
  top: -30px;
  left: 50%;
  transform: translateX(-50%);
  background-color: var(--vp-c-bg);
  color: var(--vp-c-text-1);
  border: 1px solid var(--vp-c-divider);
  border-radius: 4px;
  padding: 0.3rem 0.6rem;
  font-size: 0.8rem;
  opacity: 0;
  transition: opacity 0.2s;
  pointer-events: none;
  white-space: nowrap;
}

.copy-icon:hover .tooltip {
  opacity: 1;
}

.secret-info {
  margin-top: 1.5rem;
  font-size: 0.95rem;
  color: var(--vp-c-text-2);
  line-height: 1.5;
  background-color: rgba(125, 125, 125, 0.04);
  padding: 1rem;
  border-radius: 8px;
  border-left: 3px solid var(--vp-c-brand-light);
}

.password-strength {
  margin-top: 1.5rem;
}

.strength-meter {
  height: 8px;
  background-color: rgba(125, 125, 125, 0.1);
  border-radius: 10px;
  overflow: hidden;
  margin-bottom: 0.7rem;
}

.strength-meter div {
  height: 100%;
  border-radius: 10px;
  transition: width 0.4s ease-out;
}

.strength-level-0 {
  background-color: #e53935;
}

.strength-level-1 {
  background-color: #ff9800;
}

.strength-level-2 {
  background-color: #ffc107;
}

.strength-level-3 {
  background-color: #8bc34a;
}

.strength-level-4 {
  background-color: #4caf50;
}

.strength-text {
  font-size: 0.95rem;
  margin-bottom: 0.5rem;
  font-weight: 500;
}

.strength-0 {
  color: #e53935;
}

.strength-1 {
  color: #ff9800;
}

.strength-2 {
  color: #ffc107;
}

.strength-3 {
  color: #8bc34a;
}

.strength-4 {
  color: #4caf50;
}

.password-feedback {
  font-size: 0.9rem;
  color: var(--vp-c-text-2);
  margin-top: 0.5rem;
  font-style: italic;
}

code {
  padding: 0.2rem 0.4rem;
  border-radius: 4px;
  font-family: var(--vp-font-family-mono);
  font-size: 0.85em;
  background-color: rgba(125, 125, 125, 0.1);
}

@media (max-width: 640px) {
  .secret-type-selector {
    flex-direction: column;
  }
  
  .length-control {
    flex-direction: column;
    align-items: flex-start;
  }
  
  .length-control input[type="range"] {
    width: 100%;
  }
  
  .password-options {
    flex-direction: column;
    gap: 0.6rem;
    margin-left: 0.2rem;
  }
}
</style>
