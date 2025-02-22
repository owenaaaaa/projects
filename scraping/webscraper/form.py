#script to fill out form with data from web scraping script
from selenium import webdriver
from selenium.webdriver.common.by import By
from selenium.webdriver.common.keys import Keys
from selenium.webdriver.common.action_chains import ActionChains
from get_info import get_data, parse_address
from time import sleep

USERNAME = ""
PASSWORD = ""
CATEGORY = ""
URL = ""
SERVED = ""

#open browser
browser = webdriver.Chrome()
browser.implicitly_wait(0.5)
browser.maximize_window()

browser.get('*URL Goes Here*')

actions = ActionChains(browser)

data = get_data(URL)
title = data[0]
phone_number = data[1]
email = data[2]
address = data[3]



#login
username_field = browser.find_element(By.XPATH, 'XPATH GOES HERE')
password_field = browser.find_element(By.XPATH, 'XPATH GOES HERE')

username_field.send_keys(USERNAME)
password_field.send_keys(PASSWORD)

login_button = browser.find_element(By.XPATH, 'XPATH GOES HERE')

login_button.click()

sleep(1)

# navigate to form

vendors_link = browser.find_element(By.XPATH, 'XPATH GOES HERE')
vendors_link.click()

sleep(1)

new_vendor_button = browser.find_element(By.XPATH, 'XPATH GOES HERE')
new_vendor_button.click()

name_field = browser.find_element(By.XPATH, 'XPATH GOES HERE')
name_field.send_keys(title)


url_field = browser.find_element(By.XPATH, 'XPATH GOES HERE')
url_field.send_keys(URL)

street, town, state, zip_code = parse_address(address)
street_field = browser.find_element(By.XPATH, 'XPATH GOES HERE')
street_field.send_keys(street)

town_field = browser.find_element(By.XPATH, 'XPATH GOES HERE')
town_field.send_keys(town)

state_field = browser.find_element(By.XPATH, 'XPATH GOES HERE')
state_field.send_keys(state)

zip_field = browser.find_element(By.XPATH, 'XPATH GOES HERE')
zip_field.send_keys(zip_code)

email_field = browser.find_element(By.XPATH, 'XPATH GOES HERE')
email_field.send_keys(email)

phone_field = browser.find_element(By.XPATH, 'XPATH GOES HERE')
phone_field.send_keys(phone_number)


while True:
    finish = input('Are you done? (y/n)')
    if finish == 'y':
        break
    else:
        continue

browser.quit()