using System;
using System.Net.Http;
using System.Threading.Tasks;

public class HttpClientExample
{
    public static async Task Main(string[] args)
    {
        using (HttpClient client = new HttpClient())
        {
            try
            {
                string url = "https://jsonplaceholder.typicode.com/todos/1";
                HttpResponseMessage response = await client.GetAsync(url);
                response.EnsureSuccessStatusCode(); // Throws an exception if the HTTP status code is not 2xx
                string responseBody = await response.Content.ReadAsStringAsync();
                Console.WriteLine(responseBody);
            }
            catch (HttpRequestException e)
            {
                Console.WriteLine($"Request exception: {e.Message}");
            }
        }
    }
}
