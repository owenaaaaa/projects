from bs4 import BeautifulSoup
import re
from urllib.request import urlopen

def get_data(url):

    page = urlopen(url)
    html_bytes = page.read()
    html = html_bytes.decode("utf-8")

    soup = BeautifulSoup(html, 'html.parser')
    
    title = soup.title.string if soup.title else "Title not found"
    
    just_text = soup.get_text(strip=True)
    with_tags = soup.find_all('p', recursive=True, limit=50)

    phone_number = re.search(r'(\+\d{1,2}\s)?\(?\d{3}\)?[\s.-]?\d{3}[\s.-]?\d{4}', str(just_text))
    email = re.search(r'\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b', str(with_tags))
    address = re.search(r'(\d{1,}) [a-zA-Z0-9\s]+(\,)?( [a-zA-Z0-9]+(\,)?)+ [A-Z]{2} [0-9]{5,6}', str(with_tags).replace('<br/>', ' '))

    return title, phone_number.group() if phone_number else "Phone number not found", email.group() if email else "Email not found", address.group() if address else "Address not found"

def parse_address(address):
    # returns street, town, state, zip
    address = address.split(',')
    street = address[0]
    town = address[1].strip()
    state = address[2].strip().split(' ')[0]
    zip_code = address[2].strip().split(' ')[1]
    return street, town, state, zip_code